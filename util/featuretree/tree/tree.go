package tree

import (
	"context"
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf/util/luautil"
)

// PlainInfo 规则树配置类
type PlainInfo struct {
	FeatureType string       `json:"featureType"`
	FeatureKey  string       `json:"featureKey"`
	FeatureName string       `json:"featureName"`
	DataType    string       `json:"dataType"`
	Operator    string       `json:"operator"`
	Value       string       `json:"value"`
	And         []*PlainInfo `json:"and"`
	Or          []*PlainInfo `json:"or"`
}

// IsLeave 是否是叶子节点
func (t *PlainInfo) IsLeave() bool {
	return len(t.And) == 0 && len(t.Or) == 0
}

// FeatureTree 特征树
type FeatureTree struct {
	// Id id
	Id string `json:"id"`
	// TreePlainInfo 配置信息
	TreePlainInfo *PlainInfo `json:"-"`
	// 实际节点内容
	Node *Node `json:"node"`
}

// Node 特征树单个节点
type Node struct {
	// And and节点
	And []*Node `json:"and"`
	// Or or节点
	Or []*Node `json:"or"`
	// Leaf 叶子节点
	Leaf *Leaf `json:"leaf"`
}

// IsLeave 判断是否是叶子节点
func (t *Node) IsLeave() bool {
	return t.Leaf != nil
}

// Leaf 叶子节点
type Leaf struct {
	// FeatureType 特征类型
	FeatureType string `json:"featureType"`
	// KeyNameInfo 特征key
	KeyNameInfo *KeyNameInfo `json:"keyNameInfo"`
	// DataType 特征值类型
	DataType string `json:"dataType"`
	// Operator 操作符
	Operator *Operator `json:"operator"`
	// StringValue 期待值
	StringValue *StringValue `json:"stringValue"`
}

// Verify 校验叶子节点
func (t Leaf) Verify() error {
	if t.FeatureType == "" {
		return errors.New("wrong featureType")
	}
	_, ok := GetFetcher(t.FeatureType)
	if !ok {
		return errors.New("wrong featureFetcher")
	}
	if t.KeyNameInfo == nil {
		return errors.New("wrong KeyNameInfo")
	}
	if t.DataType == "" {
		return errors.New("wrong dataType")
	}
	_, ok = GetHandler(t.DataType)
	if !ok {
		return errors.New("wrong featureHandler")
	}
	if t.Operator == nil {
		return errors.New("wrong operator")
	}
	return nil
}

// BuildFeatureTree 构建特征树
func BuildFeatureTree(id string, info *PlainInfo) (*FeatureTree, error) {
	node := buildTreeNode(info)
	err := verifyTreeNode(node)
	if err != nil {
		return nil, err
	}
	tree := &FeatureTree{
		Id:            id,
		TreePlainInfo: info,
		Node:          node,
	}
	return tree, nil
}

// verifyTreeNode 校验节点信息
func verifyTreeNode(node *Node) error {
	if node.IsLeave() {
		err := node.Leaf.Verify()
		if err != nil {
			return err
		}
	} else {
		and := node.And
		if and != nil {
			for _, treeNode := range and {
				if err := verifyTreeNode(treeNode); err != nil {
					return err
				}
			}
		}
		or := node.Or
		if or != nil {
			for _, treeNode := range or {
				if err := verifyTreeNode(treeNode); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func buildTreeNode(info *PlainInfo) *Node {
	//如果是叶子节点 就只构建叶子节点
	if info.IsLeave() {
		var op *Operator = nil
		//find handler
		handler, ok := GetHandler(info.DataType)
		if ok {
			supportedOperators := handler.GetSupportedOperators()
			//find operator
			for _, supportedOperator := range supportedOperators {
				if supportedOperator.Operator == info.Operator {
					op = supportedOperator
				}
			}
		}
		return &Node{
			Leaf: &Leaf{
				FeatureType: info.FeatureType,
				KeyNameInfo: &KeyNameInfo{
					FeatureKey:  info.FeatureKey,
					FeatureName: info.FeatureName,
				},
				Operator: op,
				DataType: info.DataType,
				StringValue: &StringValue{
					CachedScript: luautil.NewCachedScript(fmt.Sprintf(luautil.BoolExprTemplate, info.Value)),
					Value:        info.Value,
				},
			},
		}
	} else {
		//继续递归构建节点 优先构建and
		if len(info.And) > 0 {
			nodes := make([]*Node, len(info.And))
			for i, plainInfo := range info.And {
				nodes[i] = buildTreeNode(plainInfo)
			}
			return &Node{
				And: nodes,
			}
		} else if len(info.Or) > 0 {
			nodes := make([]*Node, len(info.Or))
			for i, plainInfo := range info.Or {
				nodes[i] = buildTreeNode(plainInfo)
			}
			return &Node{
				Or: nodes,
			}
		}
	}
	return nil
}

// FeatureAnalyseContext 单次解析规则树上下文
type FeatureAnalyseContext struct {
	// FeatureTree 特征树
	FeatureTree *FeatureTree
	// currentNode 执行到哪个节点
	currentNode *Node
	// OriginMessage 原始报文
	OriginMessage map[string]any
	// featureCache 节点缓存
	featureCache map[string]any
	// Ctx
	Ctx context.Context
}

// SetCurrentNode 记录知道到哪个节点
func (f *FeatureAnalyseContext) SetCurrentNode(node *Node) {
	f.currentNode = node
}

// GetCurrentNode 获取当前节点
func (f *FeatureAnalyseContext) GetCurrentNode() *Node {
	return f.currentNode
}

// GetFeatureResultByKey 获取缓存节点
func (f *FeatureAnalyseContext) GetFeatureResultByKey(featureKey string) (any, bool) {
	feature, ok := f.featureCache[featureKey]
	if ok {
		return feature, true
	}
	return nil, false
}

// PutFeatureResult2Cache 放入缓存
func (f *FeatureAnalyseContext) PutFeatureResult2Cache(featureKey string, val any) {
	f.featureCache[featureKey] = val
}

func BuildFeatureAnalyseContext(tree *FeatureTree, originMessage map[string]any, ctx context.Context) *FeatureAnalyseContext {
	if originMessage == nil {
		originMessage = make(map[string]any)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &FeatureAnalyseContext{
		FeatureTree:   tree,
		OriginMessage: originMessage,
		Ctx:           ctx,
		featureCache:  make(map[string]any, 8),
	}
}
