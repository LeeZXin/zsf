package httptask

import (
	"context"
	"github.com/LeeZXin/zsf-utils/threadutil"
	"github.com/LeeZXin/zsf/http/httpserver"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"sync"
)

type HttpTask func(context.Context, map[string]any, url.Values)

var (
	taskMap = sync.Map{}
	token   = static.GetString("httptask.token")
)

func AppendHttpTask(name string, task HttpTask) {
	if task == nil {
		return
	}
	taskMap.Store(name, task)
}

func init() {
	httpserver.AppendRegisterRouterFunc(func(e *gin.Engine) {
		e.Any("/httpTask/v1/:taskName", func(c *gin.Context) {
			if token == "" || c.Request.Header.Get("task-token") != token {
				c.String(http.StatusForbidden, "forbidden")
				return
			}
			taskName := c.Param("taskName")
			task, b := taskMap.Load(taskName)
			if !b {
				c.String(http.StatusNotFound, "task not found")
			} else {
				body := make(map[string]any, 8)
				c.ShouldBind(&body)
				go func() {
					if err := threadutil.RunSafe(func() {
						task.(HttpTask)(c.Request.Context(), body, c.Request.URL.Query())
					}); err != nil {
						logger.Logger.Errorf("httpTask: %s err: %s", taskName, err)
					}
				}()
			}
			c.String(http.StatusOK, "ok")
		})
	})
}
