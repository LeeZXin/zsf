package nsqsrv

import (
	"context"
	"errors"
	"github.com/LeeZXin/zsf/logger"
	"github.com/gin-gonic/gin"
	"net/http"
)

var (
	DefaultAuthorization = []Authorization{
		{
			Topic:    ".*",
			Channels: []string{".*"},
			Permissions: []string{
				"subscribe",
				"publish",
			},
		},
	}
)

type Server interface {
	Auth(context.Context, AuthRequestVO) (AuthResponseVO, error)
}

type defaultServer struct {
	Token       string `json:"token"`
	Identity    string `json:"identity"`
	IdentityURL string `json:"identityURL"`
}

func NewDefaultServer(token, identity, identityURL string) Server {
	return &defaultServer{
		Token:       token,
		Identity:    identity,
		IdentityURL: identityURL,
	}
}

func (s *defaultServer) Auth(ctx context.Context, reqVO AuthRequestVO) (AuthResponseVO, error) {
	logger.Logger.WithContext(ctx).Infof("request: %v", reqVO)
	if reqVO.Secret != s.Token {
		return AuthResponseVO{}, errors.New("secret error")
	}
	return AuthResponseVO{
		TTL:            86400,
		Identity:       s.Identity,
		IdentityURL:    s.IdentityURL,
		Authorizations: DefaultAuthorization,
	}, nil
}

func HttpRouter(server Server, e *gin.Engine) error {
	if server == nil {
		return errors.New("empty server")
	}
	if e == nil {
		return errors.New("empty engine")
	}
	// 两个都行
	e.Any("/", auth(server))
	e.Any("/auth", auth(server))
	return nil
}

func auth(server Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		var reqVO AuthRequestVO
		err := c.ShouldBind(&reqVO)
		if err != nil {
			c.String(http.StatusBadRequest, "request error")
			return
		}
		responseVO, err := server.Auth(c.Request.Context(), reqVO)
		if err != nil {
			c.String(http.StatusForbidden, err.Error())
		} else {
			c.JSON(http.StatusOK, responseVO)
		}
	}
}
