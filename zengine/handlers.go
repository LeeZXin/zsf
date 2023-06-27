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

func (*ScriptHandler) Do(params *InputParams, bindings Bindings, ectx *ExecContext) (Bindings, error) {
	output := make(Bindings)
	ctx := ectx.Context()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	script, err := params.GetCompiledScript()
	if err != nil {
		return output, err
	}
	scriptRet, err := ectx.LuaExecutor().Execute(script, bindings)
	if err != nil {
		return output, err
	}
	if scriptRet.Type() == lua.LTTable {
		m, ok := ToGoValue(scriptRet).(map[string]any)
		if ok {
			output.PutAll(m)
		}
	}
	return output, nil
}
