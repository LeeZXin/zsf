package tree

import "github.com/LeeZXin/zsf/util/luautil"

// KeyNameInfo 存储key和name
type KeyNameInfo struct {
	FeatureKey  string
	FeatureName string
}

type StringValue struct {
	Value        string
	CachedScript *luautil.CachedScript
}
