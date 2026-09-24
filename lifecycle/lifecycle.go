package lifecycle

// Object 生命周期对象接口：实现该接口的组件通过 WithObjects 接入框架启动/关闭流程。
//
// 生命周期时机（由 Run 驱动，均在 main goroutine 内串行执行）：
//  1. OnApplicationStart：按 Order() 升序依次调用，完成组件初始化与启动
//     （如监听端口、连接池、后台任务）；启动失败应直接 Fatal，不留半启动状态
//  2. AfterInitialize：所有对象 OnApplicationStart 完成后调用（顺序同前），
//     用于依赖其他组件已就绪的收尾动作（如向注册中心注册服务）
//  3. OnApplicationShutdown：进程退出时由 quit 应用层钩子调用，
//     按启动顺序的逆序执行（后启动的先关），保证依赖方先于被依赖方退出
type Object interface {
	// Order 启动顺序（优先级）：值越小越先执行 OnApplicationStart/AfterInitialize，
	// 同值时保持注册顺序（稳定排序）。
	Order() int
	// OnApplicationStart 服务启动阶段回调：初始化并启动组件。
	OnApplicationStart()
	// AfterInitialize 启动后回调：所有对象启动完成后再执行的收尾逻辑。
	AfterInitialize()
	// OnApplicationShutdown 服务关闭回调：进程退出时执行优雅关闭。
	OnApplicationShutdown()
}
