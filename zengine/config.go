package zengine

import (
	"encoding/json"
	"errors"
	lua "github.com/yuin/gopher-lua"
)

// NextConfig 下一节点信息配置
type NextConfig struct {
	// Condition 下一节点执行表达式
	ConditionExpr string `json:"conditionExpr"`
	// NextNode 下一节点名称
	NextNode string `json:"nextNode"`
}

// NodeConfig 节点配置信息
type NodeConfig struct {
	// Name 节点名称 唯一标识
	Name string `json:"name"`
	// Bean 节点方法信息
	Bean BeanConfig `json:"params"`
	// Next 下一节点信息
	Next []NextConfig `json:"next"`
}

// DAGConfig 有向图
type DAGConfig struct {
	// StartNode
	StartNode string `json:"startNode"`
	// Nodes 节点信息列表
	Nodes []NodeConfig `json:"nodes"`
}

// DAG 有向图
type DAG struct {
	// startNode
	startNode string
	// nodes 节点信息列表
	nodes map[string]Node
}

func (d *DAG) StartNode() string {
	return d.startNode
}

func (d *DAG) GetNode(name string) (node Node, ok bool) {
	node, ok = d.nodes[name]
	return
}

// Node 节点信息
type Node struct {
	// Name 节点名称 唯一标识
	Name string
	// Bean 附加信息
	Bean BeanConfig
	// Next 下一节点信息
	Next []Next
}

// Next 下一节点
type Next struct {
	Condition *lua.FunctionProto
	// NextNode 下一节点名称
	NextNode string
}

func BuildDAGFromJson(jsonConfig string, luaExecutor *ScriptExecutor) (*DAG, error) {
	var c DAGConfig
	err := json.Unmarshal([]byte(jsonConfig), &c)
	if err != nil {
		return nil, err
	}
	return BuildDAG(c, luaExecutor)
}

func BuildDAG(config DAGConfig, luaExecutor *ScriptExecutor) (*DAG, error) {
	nodes, err := buildNodes(config.Nodes, luaExecutor)
	if err != nil {
		return nil, err
	}
	return &DAG{
		startNode: config.StartNode,
		nodes:     nodes,
	}, nil
}

func buildNext(config []NextConfig, luaExecutor *ScriptExecutor) ([]Next, error) {
	if config == nil {
		return nil, nil
	}
	ret := make([]Next, 0, len(config))
	for _, nextConfig := range config {
		proto, err := luaExecutor.CompileBoolLua(nextConfig.ConditionExpr)
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

func buildNode(config NodeConfig, luaExecutor *ScriptExecutor) (Node, error) {
	next, err := buildNext(config.Next, luaExecutor)
	if err != nil {
		return Node{}, err
	}
	return Node{
		Name: config.Name,
		Bean: config.Bean,
		Next: next,
	}, nil
}

func buildNodes(config []NodeConfig, luaExecutor *ScriptExecutor) (map[string]Node, error) {
	if config == nil {
		return nil, nil
	}
	ret := make(map[string]Node)
	for _, nodeConfig := range config {
		node, err := buildNode(nodeConfig, luaExecutor)
		if err != nil {
			return nil, err
		}
		ret[node.Name] = node
	}
	return ret, nil
}

type DAGExecutor struct {
	execBeans   map[string]ExecBean
	luaExecutor *ScriptExecutor
}

func NewDAGExecutor(beans []ExecBean, maxPoolSize, initPoolSize int, fnMap map[string]lua.LGFunction) *DAGExecutor {
	nodes := make(map[string]ExecBean)
	if beans != nil {
		for i := range beans {
			bean := beans[i]
			nodes[bean.GetBeanName()] = bean
		}
	}
	luaExecutor, _ := NewScriptExecutor(maxPoolSize, initPoolSize, fnMap)
	return &DAGExecutor{
		execBeans:   nodes,
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
	bean, ok := e.execBeans[node.Bean.BeanName]
	if !ok {
		return errors.New("unknown bean:" + node.Bean.BeanName)
	}
	err := bean.Do(node.Bean, ctx.GlobalBindings)
	if err != nil {
		return err
	}
	output := bean.GetOutput()
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
