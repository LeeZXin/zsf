package httptask

import (
	"github.com/LeeZXin/zsf-utils/threadutil"
	"github.com/LeeZXin/zsf/logger"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/url"
)

type Task func([]byte, url.Values)

// WithHttpTask http task api
func WithHttpTask(fns ...func() (string, Task)) gin.OptionFunc {
	taskMap := make(map[string]Task)
	for _, fn := range fns {
		name, task := fn()
		taskMap[name] = task
	}
	return func(e *gin.Engine) {
		e.Any("/httpTask/v1/:taskName", func(c *gin.Context) {
			taskName := c.Param("taskName")
			task, b := taskMap[taskName]
			if !b {
				c.String(http.StatusNotFound, "task not found")
			} else {
				defer c.Request.Body.Close()
				body, err := io.ReadAll(c.Request.Body)
				if err != nil {
					c.String(http.StatusInternalServerError, "read body failed")
					return
				}
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
	}
}
