package httptask

import (
	"github.com/LeeZXin/zsf-utils/threadutil"
	"github.com/LeeZXin/zsf/http/httpserver"
	"github.com/LeeZXin/zsf/logger"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/url"
)

type HttpTask func(io.Reader, url.Values)

var (
	taskMap = make(map[string]HttpTask, 8)
)

// AppendHttpTask 添加http task not thread-safe
func AppendHttpTask(name string, task HttpTask) {
	if task == nil {
		return
	}
	_, b := taskMap[name]
	if b {
		logger.Logger.Fatalf("duplicated http task name: %s", name)
	}
	taskMap[name] = task
}

func init() {
	httpserver.AppendRegisterRouterFunc(func(e *gin.Engine) {
		e.Any("/httpTask/v1/:taskName", func(c *gin.Context) {
			taskName := c.Param("taskName")
			task, b := taskMap[taskName]
			if !b {
				c.String(http.StatusNotFound, "task not found")
			} else {
				body := c.Request.Body
				defer body.Close()
				go func() {
					if err := threadutil.RunSafe(func() {
						task(body, c.Request.URL.Query())
					}); err != nil {
						logger.Logger.Errorf("httpTask: %s err: %s", taskName, err)
					}
				}()
				c.String(http.StatusOK, "ok")
			}
		})
	})
}
