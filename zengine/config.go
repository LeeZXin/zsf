package zengine

import (
	"github.com/spf13/cast"
	lua "github.com/yuin/gopher-lua"
)

type Args map[string]any

func (b Args) Get(key string) (any, bool) {
	ret, ok := b[key]
	return ret, ok
}

func (b Args) GetInt(key string) (int, bool) {
	ret, ok := b.Get(key)
	if ok {
		return cast.ToInt(ret), true
	}
	return 0, false
}

func (b Args) GetString(key string) (string, bool) {
	ret, ok := b.Get(key)
	if ok {
		return cast.ToString(ret), true
	}
	return "", false
}

func (b Args) GetFloat(key string) (float64, bool) {
	ret, ok := b.Get(key)
	if ok {
		return cast.ToFloat64(ret), true
	}
	return 0, false
}

func (b Args) GetBool(key string) (bool, bool) {
	ret, ok := b.Get(key)
	if ok {
		return cast.ToBool(ret), true
	}
	return false, false
}

func (b Args) Set(key string, val any) {
	b[key] = val
}

func (b Args) Del(key string) {
	delete(b, key)
}

// HandlerConfig 执行函数信息
type HandlerConfig struct {
	Name string `json:"name"`
	Args Args   `json:"args"`
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
	nodes map[string]*Node
}

func (d *DAG) StartNode() string {
	return d.startNode
}

func (d *DAG) GetNode(name string) (node *Node, ok bool) {
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
