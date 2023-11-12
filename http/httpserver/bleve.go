package httpserver

import (
	"github.com/LeeZXin/zsf-utils/ginutil"
	"github.com/LeeZXin/zsf/bleve/index"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/gin-gonic/gin"
	"net/http"
)

var (
	bleveToken  = static.GetString("logger.bleve.http.token")
	tokenHeader = "bleve-token"
)

func init() {
	if static.GetBool("logger.bleve.enabled") && static.GetBool("logger.bleve.http.enabled") {
		registerBleveLogHttpFunc()
	}
}

// registerBleveLogHttpFunc 注册http接口查看日志或删除日志
func registerBleveLogHttpFunc() {
	AppendRegisterRouterFunc(func(e *gin.Engine) {
		e.POST("/log/bleve/v1/search", func(c *gin.Context) {
			req := index.SearchBleveLogReq{}
			if checkToken(c) && ginutil.ShouldBind(&req, c) {
				logs, _, err := index.SearchBleveLog(req)
				if err != nil {
					c.String(http.StatusInternalServerError, "")
					return
				}
				c.JSON(http.StatusOK, logs)
			}
		})
		e.POST("/log/bleve/v1/clean", func(c *gin.Context) {
			req := index.CleanBleveLogReq{}
			if checkToken(c) && ginutil.ShouldBind(&req, c) {
				go func() {
					index.CleanBleveLog(req)
				}()
				c.String(http.StatusOK, "ok")
			}
		})
		e.POST("/log/bleve/v1/authenticate", func(c *gin.Context) {
			if checkToken(c) {
				c.String(http.StatusOK, "ok")
			}
		})
	})
}

func checkToken(c *gin.Context) bool {
	// 必须带个token
	if bleveToken == "" || c.Request.Header.Get(tokenHeader) != bleveToken {
		c.String(http.StatusForbidden, "invalid token")
		return false
	}
	return true
}
