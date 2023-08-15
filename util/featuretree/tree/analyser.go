package tree

import (
	"errors"
	"time"
)

// AnalyseDetail 解析详情
type AnalyseDetail struct {
	FeatureKey   string
	FeatureName  string
	Operator     *Operator
	ExpectResult string
	Result       any
}

// SingleFeatureAnalyseMetrics 单个特征key解析耗时统计
type SingleFeatureAnalyseMetrics struct {
	FeatureKey  string
	FeatureType string
	Duration    time.Duration
}

// AnalyseMetrics 特征树解析耗时统计
type AnalyseMetrics struct {
	LeafAnalyseMetrics []*SingleFeatureAnalyseMetrics
	Duration           time.Duration
}

// leafAnalyser 叶子节点解析器
type leafAnalyser struct {
	leaf *Leaf
}

// Analyse 解析叶子节点
func (t *leafAnalyser) Analyse(ctx *FeatureAnalyseContext) (*AnalyseDetail, bool, error) {
	leaf := t.leaf
	featureKey := leaf.KeyNameInfo.FeatureKey
	featureName := leaf.KeyNameInfo.FeatureName
	featureResult, err := t.getFeatureResult(featureKey, ctx)
	if err != nil {
		return nil, false, err
	}
	dataType := leaf.DataType
	handler, _ := GetHandler(dataType)
	finalResult, err := handler.Handle(leaf.StringValue, leaf.Operator, featureResult, ctx)
	if err != nil {
		return nil, false, err
	}
	if finalResult {
		return nil, true, nil
	}
	return &AnalyseDetail{
		FeatureKey:   featureKey,
		FeatureName:  featureName,
		Operator:     leaf.Operator,
		ExpectResult: leaf.StringValue.Value,
		Result:       featureResult,
	}, false, nil
}

// getFeatureResult 获取特征
func (t *leafAnalyser) getFeatureResult(featureKey string, ctx *FeatureAnalyseContext) (any, error) {
	featureResult, found := ctx.GetFeatureResultByKey(featureKey)
	if found {
		return featureResult, nil
	}
	featureType := t.leaf.FeatureType
	fetcher, _ := GetFetcher(featureType)
	ret, err := fetcher.Execute(ctx)
	if err != nil {
		return nil, err
	}
	ctx.PutFeatureResult2Cache(featureKey, ret)
	return ret, nil
}

// NodeAnalyser 节点解析器
type NodeAnalyser struct {
	FeatureAnalyseContext *FeatureAnalyseContext
	missResult            []*AnalyseDetail
	metrics               []*SingleFeatureAnalyseMetrics
}

// Analyse 解析整棵树
func (t *NodeAnalyser) Analyse() AnalyseResult {
	return t.AnalyseWithInterceptors(nil)
}

// AnalyseWithInterceptors 解析整棵树 带拦截器
func (t *NodeAnalyser) AnalyseWithInterceptors(interceptors []Interceptor) AnalyseResult {
	ctx := t.FeatureAnalyseContext.Ctx
	beginTime := time.Now()
	// 是否有超时控制
	_, hasTimeout := ctx.Deadline()
	if !hasTimeout {
		return t.doAnalyse(interceptors)
	}
	resultChan := make(chan AnalyseResult)
	defer func() {
		close(resultChan)
	}()
	go func() {
		resultChan <- t.doAnalyse(interceptors)
	}()
	select {
	case result := <-resultChan:
		return result
	case <-ctx.Done():
		//超时中断
		deadline, ok := ctx.Deadline()
		if ok {
			return &TimeoutMetricsResult{
				AnalyseMetrics: &AnalyseMetrics{
					LeafAnalyseMetrics: t.metrics,
					Duration:           time.Since(beginTime),
				},
				Timeout: deadline.Sub(beginTime),
			}
		} else {
			//context中断
			return &CancelMetricsResult{
				AnalyseMetrics: &AnalyseMetrics{
					LeafAnalyseMetrics: t.metrics,
					Duration:           time.Since(beginTime),
				},
			}
		}
	}
}

func (t *NodeAnalyser) doAnalyse(interceptors []Interceptor) AnalyseResult {
	invoker := func(ctx *FeatureAnalyseContext) AnalyseResult {
		tree := ctx.FeatureTree
		res, err := t.analyseNode(ctx, tree.Node)
		beginTime := time.Now()
		if err != nil {
			return &ErrMetricsResult{
				AnalyseMetrics: &AnalyseMetrics{
					LeafAnalyseMetrics: t.metrics,
					Duration:           time.Since(beginTime),
				},
				Err: err,
			}
		} else if res {
			return &SuccessMetricsResult{
				AnalyseMetrics: &AnalyseMetrics{
					LeafAnalyseMetrics: t.metrics,
					Duration:           time.Since(beginTime),
				},
			}
		} else {
			return &FailMetricsResult{
				AnalyseMetrics: &AnalyseMetrics{
					LeafAnalyseMetrics: t.metrics,
					Duration:           time.Since(beginTime),
				},
				AnalyseDetails: t.missResult,
			}
		}
	}
	if interceptors == nil || len(interceptors) == 0 {
		return invoker(t.FeatureAnalyseContext)
	}
	wrapper := interceptorsWrapper{
		interceptorList: interceptors,
	}
	return wrapper.intercept(t.FeatureAnalyseContext, invoker)
}

func (t *NodeAnalyser) analyseNode(fctx *FeatureAnalyseContext, node *Node) (bool, error) {
	if fctx.Ctx != nil && fctx.Ctx.Err() != nil {
		return false, fctx.Ctx.Err()
	}
	t.FeatureAnalyseContext.SetCurrentNode(node)
	beginTime := time.Now()
	if node.IsLeave() {
		//统计耗时
		defer func() {
			t.metrics = append(t.metrics, &SingleFeatureAnalyseMetrics{
				FeatureKey:  node.Leaf.KeyNameInfo.FeatureKey,
				FeatureType: node.Leaf.FeatureType,
				Duration:    time.Since(beginTime),
			})
		}()
		analyser := leafAnalyser{leaf: node.Leaf}
		analyseDetail, ok, err := analyser.Analyse(fctx)
		if err != nil {
			return false, err
		}
		if !ok {
			t.missResult = append(t.missResult, analyseDetail)
		}
		return ok, nil
	} else {
		and := node.And
		if and != nil && len(and) > 0 {
			//and节点下 全部为true
			for _, n := range and {
				b, e := t.analyseNode(fctx, n)
				if e != nil {
					return false, e
				}
				if !b {
					return false, nil
				}
			}
			return true, nil
		}
		or := node.Or
		if or != nil && len(or) > 0 {
			//and节点下 一个为true 返回true
			for _, n := range or {
				b, e := t.analyseNode(fctx, n)
				if e != nil {
					return false, e
				}
				if b {
					return true, nil
				}
			}
			return false, nil
		}
	}
	return false, errors.New("node config error")
}

// InitTreeAnalyser 初始化叶子节点解析器
func InitTreeAnalyser(featureAnalyseContext *FeatureAnalyseContext) *NodeAnalyser {
	return &NodeAnalyser{
		FeatureAnalyseContext: featureAnalyseContext,
		missResult:            make([]*AnalyseDetail, 0),
		metrics:               make([]*SingleFeatureAnalyseMetrics, 0),
	}
}
