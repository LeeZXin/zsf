package tree

import (
	"github.com/LeeZXin/zsf/util/luautil"
	"sync/atomic"
)

var (
	defaultScriptExecutor = atomic.Value{}
)

func init() {
	l, _ := luautil.NewScriptExecutor(5000, 1, nil)
	RegisterDefaultScriptExecutor(l)
}

func GetDefaultScriptExecutor() *luautil.ScriptExecutor {
	return defaultScriptExecutor.Load().(*luautil.ScriptExecutor)
}

func RegisterDefaultScriptExecutor(l *luautil.ScriptExecutor) {
	if l != nil {
		defaultScriptExecutor.Store(l)
	}
}
