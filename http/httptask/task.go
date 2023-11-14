package httptask

import (
	"github.com/LeeZXin/zsf-utils/collections/hashmap"
	"github.com/LeeZXin/zsf-utils/threadutil"
	"github.com/LeeZXin/zsf/http/httpserver"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
)

type HttpTask func(map[string]any, url.Values)

var (
	taskMap = hashmap.NewConcurrentHashMap[string, HttpTask]()
	token   = static.GetString("httptask.token")
)

func AppendHttpTask(name string, task HttpTask) {
	if task == nil {
		return
	}
	taskMap.Put(name, task)
}

func init() {
	httpserver.AppendRegisterRouterFunc(func(e *gin.Engine) {
		e.Any("/httptask/v1/:taskName", func(c *gin.Context) {
			if token == "" || c.Request.Header.Get("task-token") != token {
				c.String(http.StatusForbidden, "forbidden")
				return
			}
			taskName := c.Param("taskName")
			task, b := taskMap.Get(taskName)
			if !b {
				c.String(http.StatusNotFound, "task not found")
			} else {
				body := make(map[string]any, 8)
				c.ShouldBind(&body)
				go func() {
					if err := threadutil.RunSafe(func() {
						task(body, c.Request.URL.Query())
					}); err != nil {
						logger.Logger.Errorf("httptask: %s err: %s", taskName, err)
					}
				}()
			}
			c.String(http.StatusOK, "ok")
		})
	})
}
