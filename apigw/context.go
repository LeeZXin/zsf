package apigw

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

type apiContext struct {
	*gin.Context
	reqBody []byte
	config  RouterConfig
	header  http.Header
	url     string
}
