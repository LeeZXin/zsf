package tree

import (
	"sync"
)

var (
	fetcherMap = sync.Map{}
	handlerMap = sync.Map{}
)

type Comparator[T any] func(actual T, targets []T) bool

// FeatureFetcher 特征获取器接口
type FeatureFetcher interface {
	// GetFeatureType 获取特征处理类型
	GetFeatureType() string
	// Execute 执行获取特征
	Execute(ctx *FeatureAnalyseContext) (any, error)
}

// RegisterFetcher 注册fetcher
func RegisterFetcher(fetcher FeatureFetcher) {
	if fetcher.GetFeatureType() != "" {
		fetcherMap.Store(fetcher.GetFeatureType(), fetcher)
	}
}

// RemoveFetcher 注册Remove
func RemoveFetcher(dataType string) {
	fetcherMap.Delete(dataType)
}

// GetFetcher 获取fetcher
func GetFetcher(featureType string) (FeatureFetcher, bool) {
	value, ok := fetcherMap.Load(featureType)
	if ok {
		return value.(FeatureFetcher), true
	}
	return nil, false
}

// FeatureHandler 特征处理接口
type FeatureHandler interface {
	// GetSupportedOperators 获取支持的操作符
	GetSupportedOperators() []*Operator
	// GetDataType 支持的数据类型
	GetDataType() string
	// Handle 实际处理逻辑
	Handle(value *StringValue, operator *Operator, userValue any, ctx *FeatureAnalyseContext) (bool, error)
}

// RegisterHandler 注册handler
func RegisterHandler(handler FeatureHandler) {
	handlerMap.Store(handler.GetDataType(), handler)
}

// GetHandler 获取handler
func GetHandler(dataType string) (FeatureHandler, bool) {
	value, ok := handlerMap.Load(dataType)
	if ok {
		return value.(FeatureHandler), true
	}
	return nil, false
}
