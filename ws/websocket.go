package ws

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"nhooyr.io/websocket"
	"sync"
)

type msgWrapper struct {
	typ websocket.MessageType
	msg []byte
}

type Session struct {
	connWrapper
	extraInfo sync.Map
}

func (s *Session) GetExtraInfo(key string) (any, bool) {
	return s.extraInfo.Load(key)
}

func (s *Session) PutExtraInfo(key string, data any) {
	s.extraInfo.Store(key, data)
}

type connWrapper struct {
	conn *websocket.Conn
	ctx  context.Context
	bf   buffer
}

func (c *connWrapper) WriteTextMessage(msg string) error {
	b := c.bf.Get()
	defer c.bf.Put(b)
	b.WriteString(msg)
	return c.conn.Write(c.ctx, websocket.MessageText, b.Bytes())
}

func (c *connWrapper) WriteBinaryMessage(msg string) error {
	b := c.bf.Get()
	defer c.bf.Put(b)
	b.WriteString(msg)
	return c.conn.Write(c.ctx, websocket.MessageBinary, b.Bytes())
}

func (c *connWrapper) Close(code websocket.StatusCode, reason string) error {
	return c.conn.Close(code, reason)
}

type Service interface {
	OnOpen(*Session)
	OnTextMessage(*Session, string)
	OnBinaryMessage(*Session, []byte)
	OnClose(*Session)
	OnError(*Session, error)
}

type Config struct {
	MsgQueueSize int `json:"msgQueueSize"`
	MaxBodySize  int `json:"maxBodySize"`
}

type NewServiceFunc func() Service

func RegisterWebsocketService(newFuc NewServiceFunc, config Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !c.IsWebsocket() {
			c.String(http.StatusBadRequest, "wrong protocol")
			return
		}
		if newFuc == nil {
			c.String(http.StatusInternalServerError, "")
			return
		}
		service := newFuc()
		wsoptions := websocket.AcceptOptions{InsecureSkipVerify: true}
		conn, err := websocket.Accept(c.Writer, c.Request, &wsoptions)
		if err != nil {
			return
		}
		msgQueueSize := 64
		if config.MsgQueueSize > 0 {
			msgQueueSize = config.MsgQueueSize
		}
		msgQueue := make(chan *msgWrapper, msgQueueSize)
		ctx := c.Request.Context()
		session := &Session{
			connWrapper: connWrapper{
				conn: conn,
				ctx:  ctx,
				bf:   buffer{},
			}}
		service.OnOpen(session)
		defer func() {
			close(msgQueue)
			_ = conn.Close(websocket.StatusInternalError, "")
			service.OnClose(session)
		}()
		go func() {
			err2 := serve(service, msgQueue, session, ctx)
			checkErr(service, session, err2)
		}()
		for {
			typ, r, err3 := conn.Reader(ctx)
			if err3 != nil {
				checkErr(service, session, err3)
				return
			}
			bf := session.bf.Get()
			_, err = bf.ReadFrom(r)
			bs := bf.Bytes()
			if config.MaxBodySize > 0 && len(bs) > config.MaxBodySize {
				_ = conn.Close(websocket.StatusMessageTooBig, "message too big")
				return
			}
			msg := make([]byte, len(bs))
			copy(msg, bs)
			session.bf.Put(bf)
			msgQueue <- &msgWrapper{
				typ: typ,
				msg: msg,
			}
		}
	}
}

func checkErr(service Service, session *Session, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
		websocket.CloseStatus(err) == websocket.StatusGoingAway {
		return
	}
	service.OnError(session, err)
}

func serve(service Service, msgQueue chan *msgWrapper, session *Session, ctx context.Context) error {
	run := func(msg *msgWrapper) {
		if msg.typ == websocket.MessageBinary {
			service.OnBinaryMessage(session, msg.msg)
		} else if msg.typ == websocket.MessageText {
			service.OnTextMessage(session, string(msg.msg))
		}
	}
	for {
		select {
		case msg := <-msgQueue:
			if msg != nil {
				run(msg)
			}
			break
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
