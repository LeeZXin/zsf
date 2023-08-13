package tree

import (
	"errors"
	"github.com/LeeZXin/zsf/util/featuretree/manage"
	"github.com/LeeZXin/zsf/util/luautil"
)

type MessageFetcher struct {
}

func (m *MessageFetcher) GetFeatureType() string {
	return "message"
}

func (m *MessageFetcher) Execute(ctx *FeatureAnalyseContext) (any, error) {
	node := ctx.GetCurrentNode()
	if node == nil || !node.IsLeave() {
		return nil, errors.New("wrong node")
	}
	var (
		bindings luautil.Bindings
	)
	if ctx.OriginMessage != nil {
		bindings = luautil.Copy2Bindings(ctx.OriginMessage)
	} else {
		bindings = make(luautil.Bindings)
	}
	featureKey := node.Leaf.KeyNameInfo.FeatureKey
	result, _ := bindings.Get(featureKey)
	return result, nil
}

type ScriptFetcher struct {
}

func (m *ScriptFetcher) GetFeatureType() string {
	return "script"
}

func (m *ScriptFetcher) Execute(ctx *FeatureAnalyseContext) (any, error) {
	node := ctx.GetCurrentNode()
	if node == nil || !node.IsLeave() {
		return nil, errors.New("wrong node")
	}
	config, ok := manage.LoadScriptFeatureConfig(node.Leaf.KeyNameInfo.FeatureKey)
	if !ok {
		return nil, errors.New("nil script feature config")
	}
	proto, err := config.GetCompiledScript()
	if err != nil {
		return nil, err
	}
	return GetDefaultScriptExecutor().Execute(proto, luautil.Copy2Bindings(ctx.OriginMessage))
}

func init() {
	// 注册脚本获取特征值
	RegisterFetcher(&ScriptFetcher{})
	// 注册从报文获取特征值
	RegisterFetcher(&MessageFetcher{})
}
