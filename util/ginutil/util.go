package ginutil

import (
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/util/bizerr"
	"github.com/gin-gonic/gin"
	"net/http"
)

var (
	DefaultSuccessResp = BaseResp{
		Code:    0,
		Message: "success",
	}
)

type BaseResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func HandleErr(err error, c *gin.Context) {
	if err != nil {
		berr, ok := err.(*bizerr.Err)
		if !ok {
			logger.Logger.WithContext(c.Request.Context()).Error(err.Error())
			c.String(http.StatusInternalServerError, "系统错误")
		} else {
			c.JSON(http.StatusOK, BaseResp{
				Code:    berr.Code,
				Message: berr.Message,
			})
		}
	}
}

func BindJson(obj any, c *gin.Context) bool {
	err := c.ShouldBindJSON(obj)
	if err != nil {
		c.String(http.StatusBadRequest, "bind err")
		return false
	}
	return true
}

func GetClientIp(c *gin.Context) string {
	ip := c.ClientIP()
	if ip == "::1" {
		return "127.0.0.1"
	}
	return ip
}
