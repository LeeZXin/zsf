package zsf

type LifeCycle interface {
	// Order 加载顺序
	Order() int
	// OnApplicationStart 服务启动
	OnApplicationStart()
	// AfterInitialize 启动后
	AfterInitialize()
	// OnApplicationShutdown 服务关闭
	OnApplicationShutdown()
}
