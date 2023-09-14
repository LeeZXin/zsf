package ws

import (
	"context"
	"errors"
	"github.com/LeeZXin/zsf/util/threadutil"
	"github.com/gin-gonic/gin"
	"net/http"
	"nhooyr.io/websocket"
	"sync"
)

var (
	msgTooBigErr = errors.New("message too big")
)

type msgWrapper struct {
	typ websocket.MessageType
	msg []byte
}

type Session struct {
	request   *http.Request
	conn      *websocket.Conn
	ctx       context.Context
	buf       *bpool
	extraInfo sync.Map
	handler   *handler
}

func (s *Session) Request() *http.Request {
	return s.request
}

func (s *Session) GetExtraInfo(key string) (any, bool) {
	return s.extraInfo.Load(key)
}

func (s *Session) PutExtraInfo(key string, data any) {
	s.extraInfo.Store(key, data)
}

func (s *Session) WriteTextMessage(msg string) error {
	return s.conn.Write(s.ctx, websocket.MessageText, []byte(msg))
}

func (s *Session) WriteBinaryMessage(msg []byte) error {
	return s.conn.Write(s.ctx, websocket.MessageBinary, msg)
}

func (s *Session) Close(code websocket.StatusCode, reason string) {
	s.handler.close(code, reason)
}

type Service interface {
	OnOpen(*Session)
	OnTextMessage(*Session, string)
	OnBinaryMessage(*Session, []byte)
	OnClose(*Session)
}

type Config struct {
	MsgQueueSize int `json:"msgQueueSize"`
	MaxBodySize  int `json:"maxBodySize"`
}

type NewServiceFunc func() Service

func RegisterWebsocketService(newFunc NewServiceFunc, config Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !c.IsWebsocket() {
			c.String(http.StatusBadRequest, "wrong protocol")
			return
		}
		if newFunc == nil {
			c.String(http.StatusInternalServerError, "")
			return
		}
		service := newFunc()
		if service == nil {
			c.String(http.StatusInternalServerError, "")
			return
		}
		wsoptions := websocket.AcceptOptions{InsecureSkipVerify: true}
		conn, err := websocket.Accept(c.Writer, c.Request, &wsoptions)
		if err != nil {
			return
		}
		h := newHandler(conn, service, config, c)
		defer h.close(websocket.StatusInternalError, "system error")
		h.open()
		go h.serve()
		h.read()
	}
}

type handler struct {
	msgQueue    chan *msgWrapper
	conn        *websocket.Conn
	service     Service
	ctx         context.Context
	cancelFn    context.CancelFunc
	session     *Session
	closeOnce   sync.Once
	maxBodySize int
}

func (h *handler) serve() {
	_ = threadutil.RunSafe(func() {
		for {
			select {
			case msg, ok := <-h.msgQueue:
				if !ok {
					return
				}
				switch msg.typ {
				case websocket.MessageBinary:
					h.service.OnBinaryMessage(h.session, msg.msg)
				case websocket.MessageText:
					h.service.OnTextMessage(h.session, string(msg.msg))
				}
			case <-h.ctx.Done():
				return
			}
		}
	})
}

func (h *handler) open() {
	h.service.OnOpen(h.session)
}

func (h *handler) read() {
	_ = threadutil.RunSafe(func() {
		for {
			if h.ctx.Err() != nil {
				return
			}
			typ, reader, err := h.conn.Reader(h.ctx)
			if err != nil {
				return
			}
			buf := h.session.buf.Get()
			_, err = buf.ReadFrom(reader)
			if err != nil {
				return
			}
			bs := buf.Bytes()
			if h.maxBodySize > 0 && len(bs) > h.maxBodySize {
				h.close(websocket.StatusMessageTooBig, "message too big")
				return
			}
			msg := make([]byte, len(bs))
			copy(msg, bs)
			h.session.buf.Put(buf)
			h.msgQueue <- &msgWrapper{
				typ: typ,
				msg: msg,
			}
		}
	})
}

func (h *handler) close(code websocket.StatusCode, reason string) {
	h.closeOnce.Do(func() {
		close(h.msgQueue)
		h.cancelFn()
		_ = h.conn.Close(code, reason)
		h.service.OnClose(h.session)
	})
}

func newHandler(conn *websocket.Conn, service Service, config Config, gctx *gin.Context) *handler {
	ctx, cancelFn := context.WithCancel(context.Background())
	session := &Session{
		request: gctx.Request,
		conn:    conn,
		ctx:     ctx,
		buf:     &bpool{},
	}
	queueSize := config.MsgQueueSize
	if queueSize <= 0 {
		queueSize = 64
	}
	ret := &handler{
		msgQueue:    make(chan *msgWrapper, queueSize),
		conn:        conn,
		service:     service,
		ctx:         ctx,
		cancelFn:    cancelFn,
		session:     session,
		closeOnce:   sync.Once{},
		maxBodySize: config.MaxBodySize,
	}
	session.handler = ret
	return ret
}
