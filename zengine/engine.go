package zengine

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/spf13/cast"
	lua "github.com/yuin/gopher-lua"
	"sync"
)

const (
	// LimitTimes 限制节点递归深度，防止无限递归
	LimitTimes = 1000
)

type InputParams struct {
	HandlerConfig HandlerConfig
	//脚本缓存
	protoCache *lua.FunctionProto
	protoMu    sync.Mutex
}

// GetCompiledScript 脚本编译
func (p *InputParams) GetCompiledScript(luaExecutor *ScriptExecutor) (*lua.FunctionProto, error) {
	p.protoMu.Lock()
	defer p.protoMu.Unlock()
	if p.protoCache != nil {
		return p.protoCache, nil
	}
	script := ""
	if p.HandlerConfig.Args != nil {
		str, ok := p.HandlerConfig.Args.Get("script")
		if ok {
			script = cast.ToString(str)
		}
	}
	proto, err := luaExecutor.CompileLua(script)
	if err != nil {
		return nil, err
	}
	p.protoCache = proto
	return p.protoCache, nil
}

func NewInputParams(config HandlerConfig) *InputParams {
	return &InputParams{
		HandlerConfig: config,
	}
}

// Handler 执行节点
type Handler interface {
	// GetName 获取节点标识
	GetName() string
	// Do 执行业务逻辑的地方
	Do(*InputParams, Bindings, *ScriptExecutor, context.Context) (Bindings, error)
}

// ExecContext 单次执行上下文
type ExecContext struct {
	ctx context.Context
	// globalBindings 全局bindings
	globalBindings Bindings
}

func (e *ExecContext) GlobalBindings() Bindings {
	return e.globalBindings
}

func (e *ExecContext) Context() context.Context {
	return e.ctx
}

func NewExecContext(ctx context.Context) *ExecContext {
	return &ExecContext{
		ctx: ctx,
		// 初始化
		globalBindings: make(Bindings),
	}
}

type DAGExecutor struct {
	handlerMap  map[string]Handler
	luaExecutor *ScriptExecutor
}

func NewDAGExecutor(handlers []Handler, maxPoolSize, initPoolSize int, fnMap map[string]lua.LGFunction) *DAGExecutor {
	handlerMap := make(map[string]Handler)
	if handlers != nil {
		for i := range handlers {
			handler := handlers[i]
			handlerMap[handler.GetName()] = handler
		}
	}
	luaExecutor, _ := NewScriptExecutor(maxPoolSize, initPoolSize, fnMap)
	return &DAGExecutor{
		handlerMap:  handlerMap,
		luaExecutor: luaExecutor,
	}
}

func (d *DAGExecutor) Close() {
	d.luaExecutor.Close()
}

// Execute 执行规则引擎
func (d *DAGExecutor) Execute(dag *DAG, ctx *ExecContext) error {
	if dag == nil {
		return errors.New("nil dag")
	}
	return d.findAndExecute(dag, dag.StartNode(), ctx, 0)
}

// findAndExecute 找到节点信息并执行
func (d *DAGExecutor) findAndExecute(dag *DAG, name string, ctx *ExecContext, times int) error {
	if times > LimitTimes {
		return errors.New("out of limit")
	}
	node, ok := dag.GetNode(name)
	if !ok {
		return errors.New("unknown node: " + name)
	}
	return d.executeNode(dag, node, ctx, times)
}

// executeNode 执行节点 递归深度优先遍历
func (d *DAGExecutor) executeNode(dag *DAG, node *Node, ctx *ExecContext, times int) error {
	handler, ok := d.handlerMap[node.Params.HandlerConfig.Name]
	if !ok {
		return errors.New("unknown handler:" + node.Params.HandlerConfig.Name)
	}
	output, err := handler.Do(node.Params, ctx.GlobalBindings(), d.luaExecutor, ctx.Context())
	if err != nil {
		return err
	}
	if output != nil {
		ctx.GlobalBindings().PutAll(output)
	}
	next := node.Next
	if next != nil {
		times = times + 1
		for _, n := range next {
			res, err1 := d.luaExecutor.ExecuteAndReturnBool(n.Condition, ctx.GlobalBindings())
			if err1 != nil {
				return err1
			}
			if res {
				err1 = d.findAndExecute(dag, n.NextNode, ctx, times)
				if err1 != nil {
					return err1
				}
			}
		}
	}
	return nil
}

func (d *DAGExecutor) BuildDAGFromJson(jsonConfig string) (*DAG, error) {
	var c DAGConfig
	err := json.Unmarshal([]byte(jsonConfig), &c)
	if err != nil {
		return nil, err
	}
	return d.BuildDAG(c)
}

func (d *DAGExecutor) BuildDAG(config DAGConfig) (*DAG, error) {
	nodes, err := d.buildNodes(config.Nodes)
	if err != nil {
		return nil, err
	}
	return &DAG{
		startNode: config.StartNode,
		nodes:     nodes,
	}, nil
}

func (d *DAGExecutor) buildNext(config []NextConfig) ([]Next, error) {
	if config == nil {
		return nil, nil
	}
	ret := make([]Next, 0, len(config))
	for _, nextConfig := range config {
		proto, err := d.luaExecutor.CompileBoolLua(nextConfig.ConditionExpr)
		if err != nil {
			return nil, err
		}
		ret = append(ret, Next{
			Condition: proto,
			NextNode:  nextConfig.NextNode,
		})
	}
	return ret, nil
}

func (d *DAGExecutor) buildNode(config NodeConfig) (*Node, error) {
	next, err := d.buildNext(config.Next)
	if err != nil {
		return nil, err
	}
	return &Node{
		Name:   config.Name,
		Params: NewInputParams(config.Handler),
		Next:   next,
	}, nil
}

func (d *DAGExecutor) buildNodes(config []NodeConfig) (map[string]*Node, error) {
	if config == nil {
		return make(map[string]*Node), nil
	}
	ret := make(map[string]*Node)
	for _, nodeConfig := range config {
		node, err := d.buildNode(nodeConfig)
		if err != nil {
			return nil, err
		}
		ret[node.Name] = node
	}
	return ret, nil
}
