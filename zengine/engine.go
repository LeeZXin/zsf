package zengine

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LeeZXin/zsf/util/luautil"
	"strconv"
)

type InputParams struct {
	HandlerConfig HandlerConfig
	//脚本缓存
	luautil.CachedScript
}

func NewInputParams(config HandlerConfig) *InputParams {
	script := ""
	val, ok := config.Args.GetString("script")
	if ok {
		script = val
	}
	return &InputParams{
		HandlerConfig: config,
		CachedScript:  luautil.NewCachedScript(script),
	}
}

// Handler 执行节点
type Handler interface {
	// GetName 获取节点标识
	GetName() string
	//Do 执行业务逻辑的地方
	Do(*InputParams, luautil.Bindings, *ExecContext) (luautil.Bindings, error)
}

// ExecContext 单次执行上下文
type ExecContext struct {
	globalBindings luautil.Bindings
	ctx            context.Context
	luaExecutor    *ScriptExecutor
}

func (e *ExecContext) Context() context.Context {
	return e.ctx
}

func (e *ExecContext) GlobalBindings() luautil.Bindings {
	return e.globalBindings
}

func (e *ExecContext) LuaExecutor() *ScriptExecutor {
	return e.luaExecutor
}

type DAGExecutor struct {
	handlerMap  map[string]Handler
	luaExecutor *ScriptExecutor
	limitTimes  int
}

func NewDAGExecutor(handlers []Handler, luaExecutor *ScriptExecutor, limitTimes int) *DAGExecutor {
	handlerMap := make(map[string]Handler)
	if handlers != nil {
		for i := range handlers {
			handler := handlers[i]
			handlerMap[handler.GetName()] = handler
		}
	}
	if luaExecutor == nil {
		luaExecutor, _ = NewScriptExecutor(1000, 1, nil)
	}
	if limitTimes <= 0 {
		limitTimes = 10000
	}
	return &DAGExecutor{
		handlerMap:  handlerMap,
		luaExecutor: luaExecutor,
		limitTimes:  limitTimes,
	}
}

func (d *DAGExecutor) NewExecContext(ctx context.Context) *ExecContext {
	if ctx == nil {
		ctx = context.Background()
	}
	return &ExecContext{
		globalBindings: luautil.NewBindings(),
		ctx:            ctx,
		luaExecutor:    d.luaExecutor,
	}
}

func (d *DAGExecutor) Close() {
	d.luaExecutor.Close()
}

// Execute 执行规则引擎
func (d *DAGExecutor) Execute(dag *DAG, ectx *ExecContext) error {
	if dag == nil {
		return errors.New("nil dag")
	}
	return d.findAndExecute(dag, dag.StartNode(), ectx, 0)
}

// findAndExecute 找到节点信息并执行
func (d *DAGExecutor) findAndExecute(dag *DAG, name string, ectx *ExecContext, times int) error {
	if ectx.ctx.Err() != nil {
		return ectx.ctx.Err()
	}
	if times > d.limitTimes {
		return errors.New("out of limit: " + strconv.Itoa(d.limitTimes))
	}
	node, ok := dag.GetNode(name)
	if !ok {
		return errors.New("unknown node: " + name)
	}
	return d.executeNode(dag, node, ectx, times)
}

// executeNode 执行节点 递归深度优先遍历
func (d *DAGExecutor) executeNode(dag *DAG, node Node, ectx *ExecContext, times int) error {
	handler, ok := d.handlerMap[node.Params.HandlerConfig.Name]
	if !ok {
		return errors.New("unknown handler:" + node.Params.HandlerConfig.Name)
	}
	output, err := handler.Do(node.Params, ectx.GlobalBindings(), ectx)
	if err != nil {
		return err
	}
	if output != nil {
		ectx.GlobalBindings().PutAll(output)
	}
	next := node.Next
	if next != nil {
		times = times + 1
		for _, n := range next {
			res, err := d.luaExecutor.ExecuteAndReturnBool(n.Condition, ectx.GlobalBindings())
			if err != nil {
				return err
			}
			if res {
				err = d.findAndExecute(dag, n.NextNode, ectx, times)
				if err != nil {
					return err
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

func (d *DAGExecutor) buildNode(config NodeConfig) (Node, error) {
	next, err := d.buildNext(config.Next)
	if err != nil {
		return Node{}, err
	}
	return Node{
		Name:   config.Name,
		Params: NewInputParams(config.Handler),
		Next:   next,
	}, nil
}

func (d *DAGExecutor) buildNodes(config []NodeConfig) (map[string]Node, error) {
	if config == nil {
		return make(map[string]Node), nil
	}
	ret := make(map[string]Node)
	for _, nodeConfig := range config {
		node, err := d.buildNode(nodeConfig)
		if err != nil {
			return nil, err
		}
		ret[node.Name] = node
	}
	return ret, nil
}
