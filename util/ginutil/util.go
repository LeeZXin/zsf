package ginutil

import (
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/util/bizerr"
	"github.com/LeeZXin/zsf/util/runtimeutil"
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
			logger.Logger.WithContext(c.Request.Context()).Error(err.Error() + "\n" + runtimeutil.PrettyErrCallerTrace(10))
			c.String(http.StatusInternalServerError, "系统错误")
		} else {
			c.JSON(http.StatusOK, BaseResp{
				Code:    berr.Code,
				Message: berr.Message,
			})
		}
	}
}

func ShouldBind(obj any, c *gin.Context) bool {
	err := c.ShouldBind(obj)
	if err != nil {
		c.String(http.StatusBadRequest, "request format err")
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

func RetHttpJson(result any, c *gin.Context) {
	c.JSON(http.StatusOK, result)
}
