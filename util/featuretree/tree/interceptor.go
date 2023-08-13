package tree

type Invoker func(*FeatureAnalyseContext) AnalyseResult

type Interceptor func(*FeatureAnalyseContext, Invoker) AnalyseResult

// 拦截器wrapper 实现类似洋葱递归执行功能
type interceptorsWrapper struct {
	interceptorList []Interceptor
}

func (i *interceptorsWrapper) intercept(ctx *FeatureAnalyseContext, invoker Invoker) AnalyseResult {
	if i.interceptorList == nil || len(i.interceptorList) == 0 {
		return invoker(ctx)
	}
	return i.recursive(0, ctx, invoker)
}

func (i *interceptorsWrapper) recursive(index int, ctx *FeatureAnalyseContext, invoker Invoker) AnalyseResult {
	return i.interceptorList[index](ctx, func(ctx *FeatureAnalyseContext) AnalyseResult {
		if index == len(i.interceptorList)-1 {
			return invoker(ctx)
		} else {
			return i.recursive(index+1, ctx, invoker)
		}
	})
}
