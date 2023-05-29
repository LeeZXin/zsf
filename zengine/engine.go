package zengine

import (
	"errors"
	lua "github.com/yuin/gopher-lua"
)

type Params struct {
	HandlerConfig
}

func NewParams(config HandlerConfig) *Params {
	return &Params{
		HandlerConfig: config,
	}
}

// Handler 执行节点
type Handler interface {
	// GetName 获取节点标识
	GetName() string
	//Do 执行业务逻辑的地方
	Do(*Params, Bindings) (Bindings, error)
}

// ExecContext 单次执行上下文
type ExecContext struct {
	GlobalBindings Bindings
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

// Execute 执行规则引擎
func (e *DAGExecutor) Execute(dag *DAG, ctx *ExecContext) error {
	if dag == nil {
		return errors.New("nil dag")
	}
	return e.findAndExecute(dag, dag.StartNode(), ctx)
}

// findAndExecute 找到节点信息并执行
func (e *DAGExecutor) findAndExecute(dag *DAG, name string, ctx *ExecContext) error {
	startNode, ok := dag.GetNode(name)
	if !ok {
		return errors.New("unknown node: " + name)
	}
	return e.executeNode(dag, startNode, ctx)
}

// executeNode 执行节点 递归深度优先遍历
func (e *DAGExecutor) executeNode(dag *DAG, node Node, ctx *ExecContext) error {
	handler, ok := e.handlerMap[node.Params.Name]
	if !ok {
		return errors.New("unknown handler:" + node.Params.Name)
	}
	output, err := handler.Do(node.Params, ctx.GlobalBindings)
	if err != nil {
		return err
	}
	if output != nil {
		ctx.GlobalBindings.PutAll(output)
	}
	next := node.Next
	if next != nil {
		for _, n := range next {
			res, err := e.luaExecutor.ExecuteAndReturnBool(n.Condition, ctx.GlobalBindings)
			if err != nil {
				return err
			}
			if res {
				err = e.findAndExecute(dag, n.NextNode, ctx)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

/*
{
	"startNode": {
		"name": "A",
		"params": {
			"url": "http://aaa/aa/aa",
			"request": `{"userId": "xxxx"}`
		},
		"next": [
			{
				"conditionExpr": "A.result == 1",
				"nextNode": "B"
			},
			{
				"conditionExpr": "A.result == 2",
				"nextNode": "B"
			}
		]
	},

}
*/
