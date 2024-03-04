package zsf

type LifeCycle interface {
	// OnApplicationStart 服务启动
	OnApplicationStart()
	// AfterInitialize 启动后
	AfterInitialize()
	// OnApplicationShutdown 服务关闭
	OnApplicationShutdown()
}

func RegisterApplicationLifeCycle(lifeCycle LifeCycle) {
	//if lifeCycle == nil {
	//	return
	//}
	//mu.Lock()
	//defer mu.Unlock()
	//lifeCycles = append(lifeCycles, lifeCycle)
}
