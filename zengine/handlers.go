package zengine

import (
	"github.com/LeeZXin/zsf/util/luautil"
	lua "github.com/yuin/gopher-lua"
)

// ScriptHandler 脚本执行节点
type ScriptHandler struct {
}

func (*ScriptHandler) GetName() string {
	return "scriptNode"
}

func (*ScriptHandler) Do(params *InputParams, bindings luautil.Bindings, ectx *ExecContext) (luautil.Bindings, error) {
	output := luautil.NewBindings()
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
		m, ok := luautil.ToGoValue(scriptRet).(map[string]any)
		if ok {
			output.PutAll(m)
		}
	}
	return output, nil
}
