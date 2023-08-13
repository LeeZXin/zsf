package zengine

import (
	"errors"
	"github.com/LeeZXin/zsf/util/luautil"
	lua "github.com/yuin/gopher-lua"
)

// 使用gopher-lua来做规则引擎的脚本配置以及条件转发

// ScriptExecutor 脚本执行器
type ScriptExecutor struct {
	pool *luautil.LStatePool
}

// NewScriptExecutor 构建执行器
func NewScriptExecutor(maxSize int, initSize int, fnMap map[string]lua.LGFunction) (*ScriptExecutor, error) {
	pool, err := luautil.NewLStatePool(maxSize, initSize, fnMap)
	if err != nil {
		return nil, err
	}
	return &ScriptExecutor{pool: pool}, nil
}

// CompileBoolLua 编译布尔表达式lua
func (e *ScriptExecutor) CompileBoolLua(x string) (*lua.FunctionProto, error) {
	return luautil.CompileBoolLua(x)
}

// CompileLua 编译lua脚本
func (e *ScriptExecutor) CompileLua(x string) (*lua.FunctionProto, error) {
	return luautil.CompileLua(x)
}

// Execute 执行lua脚本 仅返回单个返回值
func (e *ScriptExecutor) Execute(proto *lua.FunctionProto, bindings luautil.Bindings) (lua.LValue, error) {
	L := e.pool.Get()
	defer e.pool.Put(L)
	args, err := luautil.Execute(L, proto, bindings)
	if err != nil {
		return nil, err
	}
	if len(args) > 0 {
		return args[0], nil
	}
	return lua.LNil, nil
}

func (e *ScriptExecutor) ExecuteAndReturnBool(proto *lua.FunctionProto, bindings luautil.Bindings) (bool, error) {
	res, err := e.Execute(proto, bindings)
	if err != nil {
		return false, err
	}
	b, ok := res.(lua.LBool)
	if ok {
		return bool(b), nil
	}
	return false, errors.New("unsupported result")
}

func (e *ScriptExecutor) Close() {
	e.pool.CloseAll()
}
