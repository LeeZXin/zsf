package zengine

type BeanConfig struct {
	BeanName string
	Params   map[string]any
}

// ExecBean 执行节点
type ExecBean interface {
	// GetBeanName 获取节点标识
	GetBeanName() string
	//Do 执行业务逻辑的地方
	Do(BeanConfig, Bindings) error
	// GetOutput 执行完要放入全局变量的数据 往其他节点传递
	GetOutput() Bindings
}

// ExecContext 单次执行上下文
type ExecContext struct {
	GlobalBindings Bindings
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
