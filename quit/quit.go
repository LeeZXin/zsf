/*
Package quit 提供优雅关闭（Graceful Shutdown）机制。

收到 SIGINT/SIGTERM 后先关闭 Stopping()（后台循环据此退出），再按四段
串行执行钩子——顺序对齐「先停流量、再释放依赖」：

	应用钩子（Application）:
	  最先执行：停 cron 新任务、各 Object.OnApplicationShutdown
	  （摘注册、关闭 HTTP/gRPC 监听并等待在途请求）

	高优先级钩子（High）:
	  流量停稳后清理运行时：gopool.Close、chunktask.Sync、
	  关闭 gRPC/NATS 客户端

	低优先级钩子（Low）:
	  释放仍被业务依赖的基础设施：数据库连接池、选主租约

	最终钩子（Final）:
	  收尾：Loki/指标冲刷、Nacos client、sqlite replica

钩子按注册顺序执行、同级不并发。单个钩子应自行控制超时；出错应记日志
并继续，部分关闭总比不关闭好。
*/
package quit

import (
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"
)

// DefaultShutdownTimeout 服务端优雅关闭的默认超时（HTTP/gRPC/MCP Shutdown）。
const DefaultShutdownTimeout = 30 * time.Second

// ShutdownTimeout 将配置读到的 duration 规整为有效超时：未配置或非法时用默认值。
func ShutdownTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultShutdownTimeout
	}
	return d
}

var (
	mu sync.Mutex

	applicationHooks  []func()
	highPriorityHooks []func()
	lowPriorityHooks  []func()
	finalHooks        []func()

	stoppingOnce sync.Once
	stopping     = make(chan struct{})
)

func addHook(dst *[]func(), hook func()) {
	if hook == nil {
		return
	}
	mu.Lock()
	*dst = append(*dst, hook)
	mu.Unlock()
}

func snapshotHooks() (app, high, low, final []func()) {
	mu.Lock()
	defer mu.Unlock()
	return slices.Clone(applicationHooks),
		slices.Clone(highPriorityHooks),
		slices.Clone(lowPriorityHooks),
		slices.Clone(finalHooks)
}

func runHooks(hooks []func()) {
	for _, fn := range hooks {
		fn()
	}
}

func signalStopping() {
	stoppingOnce.Do(func() { close(stopping) })
}

// Stopping 在 Wait 收到退出信号后立即关闭，早于任何钩子。
// 后台循环（AddLoop、SubscribeForever、选主重试）应 select 该通道以便尽快退出。
func Stopping() <-chan struct{} {
	return stopping
}

// AddApplicationShutdownHook 注册应用层关闭钩子（最先执行）：停调度、关监听。
func AddApplicationShutdownHook(hook func()) {
	addHook(&applicationHooks, hook)
}

// AddHighPriorityShutdownHook 注册高优先级关闭钩子：流量停稳后的运行时清理
// （协程池、客户端连接、批量刷盘）。
func AddHighPriorityShutdownHook(hook func()) {
	addHook(&highPriorityHooks, hook)
}

// AddLowPriorityShutdownHook 注册低优先级关闭钩子：数据库、租约等基础设施。
func AddLowPriorityShutdownHook(hook func()) {
	addHook(&lowPriorityHooks, hook)
}

// AddFinalShutdownHook 注册最终关闭钩子：日志/指标冲刷与远端 client 关闭。
func AddFinalShutdownHook(hook func()) {
	addHook(&finalHooks, hook)
}

/*
Wait 注册信号监听并阻塞，直到收到 SIGINT 或 SIGTERM。

收到信号后的执行顺序：
 1. 关闭 Stopping()
 2. 应用钩子
 3. 高优先级钩子
 4. 低优先级钩子
 5. 最终钩子
 6. 返回（main 退出）

应在 main goroutine 中调用。关闭期间再次信号只会进入缓冲通道被丢弃，
二次强退由外部编排（如 K8s terminationGracePeriod）决定。
*/
func Wait() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	signalStopping()
	app, high, low, final := snapshotHooks()
	runHooks(app)
	runHooks(high)
	runHooks(low)
	runHooks(final)
}
