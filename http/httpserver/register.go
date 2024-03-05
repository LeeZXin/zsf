package httpserver

import (
	"github.com/LeeZXin/zsf/property/static"
	"github.com/gin-gonic/gin"
)

type RegisterRouterFunc func(*gin.Engine)

var (
	registerFuncList = make([]RegisterRouterFunc, 0)
)

var (
	filters = make([]gin.HandlerFunc, 0)
)

func init() {
	// 禁用filter
	if static.GetBool("application.disableMicro") {
		AppendFilters(recoverFilter())
	} else {
		AppendFilters(
			recoverFilter(),
			headerFilter(),
			promFilter(),
		)
	}
}

func AppendRegisterRouterFunc(f ...RegisterRouterFunc) {
	if len(f) == 0 {
		return
	}
	registerFuncList = append(registerFuncList, f...)
}

func getRegisterFuncList() []RegisterRouterFunc {
	ret := registerFuncList[:]
	// for gc
	registerFuncList = nil
	return ret
}

func AppendFilters(f ...gin.HandlerFunc) {
	if len(f) == 0 {
		return
	}
	filters = append(filters, f...)
}

func getFilters() []gin.HandlerFunc {
	ret := filters[:]
	// for gc
	filters = nil
	return ret
}
