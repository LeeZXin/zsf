package zengine

import (
	lua "github.com/yuin/gopher-lua"
)

// ScriptHandler 脚本执行节点
type ScriptHandler struct {
}

func (*ScriptHandler) GetName() string {
	return "scriptNode"
}

func (*ScriptHandler) Do(params *InputParams, luaExecutor *ScriptExecutor, ctx *ExecContext) (Bindings, error) {
	output := make(Bindings)
	script, err := params.GetCompiledScript(luaExecutor)
	if err != nil {
		return output, err
	}
	scriptRet, err := luaExecutor.Execute(script, ctx.GlobalBindings())
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
