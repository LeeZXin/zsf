package zengine

import (
	"errors"
	"fmt"
	"github.com/spf13/cast"
	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
	"reflect"
	"strings"
	"sync"
)

// 使用gopher-lua来做规则引擎的脚本配置以及条件转发
// 下面封装的LStatePool 和相关map[string]any与LTable的相互转化

var (
	// BoolExprTemplate 仅返回布尔表达式的lua template
	// ret = params.id == 1
	// return ret
	BoolExprTemplate = `
		print(params.gg)
		ret = %s
		return ret
	`
)

// Bindings lua global
type Bindings map[string]any

func (b Bindings) Get(key string) (any, bool) {
	ret, ok := b[key]
	return ret, ok
}

func (b Bindings) GetInt(key string) (int, bool) {
	ret, ok := b.Get(key)
	if ok {
		return cast.ToInt(ret), true
	}
	return 0, false
}

func (b Bindings) GetString(key string) (string, bool) {
	ret, ok := b.Get(key)
	if ok {
		return cast.ToString(ret), true
	}
	return "", false
}

func (b Bindings) GetFloat(key string) (float64, bool) {
	ret, ok := b.Get(key)
	if ok {
		return cast.ToFloat64(ret), true
	}
	return 0, false
}

func (b Bindings) GetBool(key string) (bool, bool) {
	ret, ok := b.Get(key)
	if ok {
		return cast.ToBool(ret), true
	}
	return false, false
}

func (b Bindings) Set(key string, val any) {
	b[key] = val
}

func (b Bindings) Del(key string) {
	delete(b, key)
}

func (b Bindings) PutAll(data map[string]any) {
	if data != nil {
		for k, v := range data {
			b[k] = v
		}
	}
}

// ToLTable 转化为LTable
func (b Bindings) ToLTable(L *lua.LState) *lua.LTable {
	value := fromGoValue(b, L)
	return value.(*lua.LTable)
}

// FromLTable 从ltable获取数据
func (b Bindings) FromLTable(table *lua.LTable) error {
	if table == nil {
		return errors.New("nil table")
	}
	value := ToGoValue(table).(map[string]any)
	for k, v := range value {
		b[k] = v
	}
	return nil
}

// fromGoValue go转LValue 只处理基本类型参数 func chan等不处理
func fromGoValue(v any, L *lua.LState) lua.LValue {
	if v == nil {
		return lua.LNil
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.String:
		return lua.LString(cast.ToString(v))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fallthrough
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		fallthrough
	case reflect.Float32, reflect.Float64:
		return lua.LNumber(cast.ToFloat64(v))
	case reflect.Bool:
		return lua.LBool(cast.ToBool(v))
	case reflect.Map:
		table := L.NewTable()
		// 只认key类型为string的map
		if r.Type().Key().Kind() == reflect.String {
			keys := r.MapKeys()
			for _, key := range keys {
				value := r.MapIndex(key).Interface()
				table.RawSet(lua.LString(key.String()), fromGoValue(value, L))
			}
		}
		return table
	case reflect.Slice, reflect.Array:
		table := L.NewTable()
		for i := 0; i < r.Len(); i++ {
			table.Append(fromGoValue(r.Index(i).Interface(), L))
		}
		return table
	case reflect.Struct:
		table := L.NewTable()
		for i := 0; i < r.NumField(); i++ {
			fk := r.Type().Field(i).Name
			fv := fromGoValue(r.Field(i).Interface(), L)
			table.RawSet(lua.LString(fk), fv)
		}
		return table
	case reflect.Pointer:
		if !r.IsNil() {
			return fromGoValue(r.Elem().Interface(), L)
		}
	}
	return lua.LNil
}

// ToGoValue LValue转go对象
func ToGoValue(lv lua.LValue) any {
	switch v := lv.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(v)
	case lua.LString:
		return string(v)
	case lua.LNumber:
		return float64(v)
	case *lua.LTable:
		maxn := v.MaxN()
		if maxn == 0 { // table
			ret := make(map[string]any)
			v.ForEach(func(key, value lua.LValue) {
				ret[key.String()] = ToGoValue(value)
			})
			return ret
		} else { // array
			ret := make([]any, 0, maxn)
			for i := 1; i <= maxn; i++ {
				ret = append(ret, ToGoValue(v.RawGetInt(i)))
			}
			return ret
		}
	default:
		return v
	}
}

// lStatePool LState池 复用LState
type lStatePool struct {
	mu   sync.Mutex
	pool []*lua.LState
	// 限制最大数量
	maxSize int
	// 初始化数量
	initSize int
	// 默认注册使用的go函数 不允许name为params
	globalFn map[string]lua.LGFunction
}

// newLStatePool 构建池
func newLStatePool(maxSize int, initSize int, fnMap map[string]lua.LGFunction) (*lStatePool, error) {
	if maxSize <= 0 {
		return nil, errors.New("maxSize should greater than 0")
	}
	if maxSize < initSize {
		return nil, errors.New("initSize should less than maxSize")
	}
	if fnMap == nil {
		fnMap = make(map[string]lua.LGFunction)
	}
	var pool []*lua.LState
	if initSize <= 0 {
		pool = make([]*lua.LState, 0)
	} else {
		pool = make([]*lua.LState, 0, initSize)
	}
	return &lStatePool{
		mu:       sync.Mutex{},
		pool:     pool,
		maxSize:  maxSize,
		initSize: initSize,
		globalFn: fnMap,
	}, nil
}

func (p *lStatePool) init() {
	if p.initSize > 0 {
		for i := 0; i < p.initSize; i++ {
			p.pool = append(p.pool, p.newLState())
		}
	}
}

func (p *lStatePool) newLState() *lua.LState {
	L := lua.NewState()
	if len(p.globalFn) > 0 {
		for name, fn := range p.globalFn {
			// 注册函数
			L.SetGlobal(name, L.NewFunction(fn))
		}
	}
	return L
}

func (p *lStatePool) Put(state *lua.LState) {
	if state != nil {
		p.mu.Lock()
		defer p.mu.Unlock()
		if len(p.pool) >= p.maxSize {
			state.Close()
		} else {
			p.pool = append(p.pool, state)
		}
	}
}

func (p *lStatePool) Get() *lua.LState {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.pool) > 0 {
		res := p.pool[0]
		p.pool = p.pool[1:]
		return res
	}
	return p.newLState()
}

// CloseAll 关闭所有的LState
func (p *lStatePool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, state := range p.pool {
		state.Close()
	}
	p.pool = make([]*lua.LState, 0)
}

// ScriptExecutor 脚本执行器
type ScriptExecutor struct {
	pool *lStatePool
}

// NewScriptExecutor 构建执行器
func NewScriptExecutor(maxSize int, initSize int, fnMap map[string]lua.LGFunction) (*ScriptExecutor, error) {
	pool, err := newLStatePool(maxSize, initSize, fnMap)
	if err != nil {
		return nil, err
	}
	return &ScriptExecutor{pool: pool}, nil
}

// CompileBoolLua 编译布尔表达式lua
func (e *ScriptExecutor) CompileBoolLua(x string) (*lua.FunctionProto, error) {
	x = fmt.Sprintf(BoolExprTemplate, x)
	return e.CompileLua(x)
}

// CompileLua 编译lua脚本
func (e *ScriptExecutor) CompileLua(x string) (*lua.FunctionProto, error) {
	chunk, err := parse.Parse(strings.NewReader(x), "<script>")
	if err != nil {
		return nil, err
	}
	return lua.Compile(chunk, "<script>")
}

// Execute 执行lua脚本 仅返回单个返回值
func (e *ScriptExecutor) Execute(proto *lua.FunctionProto, bindings Bindings) (lua.LValue, error) {
	if proto == nil {
		return nil, errors.New("nil proto")
	}
	if bindings == nil {
		bindings = make(Bindings)
	}
	L := e.pool.Get()
	defer e.pool.Put(L)
	// 默认入参的变量为params
	params := bindings.ToLTable(L)
	L.SetGlobal("params", params)
	err := L.CallByParam(lua.P{
		Fn:      L.NewFunctionFromProto(proto),
		NRet:    lua.MultRet,
		Protect: true,
	})
	if err != nil {
		return nil, err
	}
	value := L.Get(-1)
	if value != nil && value.Type() != lua.LTNil {
		L.Pop(1)
	}
	return value, nil
}

func (e *ScriptExecutor) ExecuteAndReturnBool(proto *lua.FunctionProto, bindings Bindings) (bool, error) {
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
