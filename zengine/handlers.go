package zengine

import (
	"context"
	lua "github.com/yuin/gopher-lua"
)

// ScriptHandler 脚本执行节点
type ScriptHandler struct {
}

func (*ScriptHandler) GetName() string {
	return "scriptNode"
}

func (*ScriptHandler) Do(params *InputParams, bindings Bindings, luaExecutor *ScriptExecutor, ctx context.Context) (Bindings, error) {
	output := make(Bindings)
	script, err := params.GetCompiledScript(luaExecutor)
	if err != nil {
		return output, err
	}
	scriptRet, err := luaExecutor.Execute(script, bindings)
	if err != nil {
		return output, err
	}
  	// 对脚本返回值进行处理
	if scriptRet.Type() == lua.LTTable {
		m, ok := ToGoValue(scriptRet).(map[string]any)
		if ok {
			output.PutAll(m)
		}
	}
	return output, nil
}
