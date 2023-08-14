package zengine

import (
	"github.com/LeeZXin/zsf/util/luautil"
	lua "github.com/yuin/gopher-lua"
)

// HandlerConfig 执行函数信息
type HandlerConfig struct {
	Name string           `json:"name"`
	Args luautil.Bindings `json:"args"`
}

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
	Handler HandlerConfig `json:"handler"`
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
	// Params 附加信息
	Params *InputParams
	// Next 下一节点信息
	Next []Next
}

// Next 下一节点
type Next struct {
	Condition *lua.FunctionProto
	// NextNode 下一节点名称
	NextNode string
}
