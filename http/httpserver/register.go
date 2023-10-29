package httpserver

import (
	"github.com/LeeZXin/zsf/property/static"
	"github.com/gin-gonic/gin"
	"sync"
)

type RegisterRouterFunc func(*gin.Engine)

var (
	registerFuncList = make([]RegisterRouterFunc, 0)
	registerFuncMu   = sync.Mutex{}
)

var (
	filters  = make([]gin.HandlerFunc, 0)
	filterMu = sync.Mutex{}
)

func init() {
	// 禁用filter
	if static.GetBool("application.disableMicro") {
		AppendFilters(recoverFilter())
	} else {
		AppendFilters(
			recoverFilter(),
			actuatorFilter(),
			headerFilter(),
			promFilter(),
			skywalkingFilter(),
		)
	}
}

func AppendRegisterRouterFunc(f ...RegisterRouterFunc) {
	if len(f) == 0 {
		return
	}
	registerFuncMu.Lock()
	defer registerFuncMu.Unlock()
	registerFuncList = append(registerFuncList, f...)
}

func getRegisterFuncList() []RegisterRouterFunc {
	registerFuncMu.Lock()
	defer registerFuncMu.Unlock()
	return registerFuncList[:]
}

func AppendFilters(f ...gin.HandlerFunc) {
	if len(f) == 0 {
		return
	}
	filterMu.Lock()
	defer filterMu.Unlock()
	filters = append(filters, f...)
}

func getFilters() []gin.HandlerFunc {
	filterMu.Lock()
	defer filterMu.Unlock()
	return filters[:]
}