// Package lifecycle 提供服务生命周期编排：定义 Object 接口并统一驱动启动、初始化与关闭流程。
//
// 启动时序（Run 在 main goroutine 中串行执行）：
//  1. 执行 start 包注册的初始化钩子（start.AddInit，先于一切对象启动）
//  2. 调用 WithObjects / WithObjectsList 注册的构造器（此时配置等框架状态已就绪）
//  3. 按 Order() 升序（同值保序）依次调用各对象的 OnApplicationStart
//  4. 注册关闭钩子：退出时按启动逆序调用 OnApplicationShutdown（应用层钩子，先于连接池/DB）
//  5. 按启动顺序调用各对象的 AfterInitialize
//  6. 阻塞在 quit.Wait()，等待 SIGINT/SIGTERM 后执行关闭钩子并返回
//
// 失败语义：任何对象的启动失败应自行 Fatal（fail-fast），
// 框架不做回滚，避免进程带病运行。
package lifecycle

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"

	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/start"
)

// Run 是服务启动入口，阻塞直到进程收到 SIGINT/SIGTERM 信号并完成优雅关闭。
//
// 参数: options - 启动配置（WithObjects/WithObjectsList 指定生命周期对象，
// WithObjectsList 延迟构造需读配置的对象，WithPidFile 写 PID 文件；
// 对象启动前的全局初始化经 start.AddInit 注册，不经 options 传入）
//
// 注意：
//   - 必须在 main goroutine 中调用（内部阻塞并等待信号）
//   - 对象启动失败若未自行 Fatal，框架仍会继续后续流程，各对象须保证失败即退出
//   - PID 文件只写入不删除（覆盖式写入），由外部进程管理清理
func Run(options ...Option) {
	o := new(option)
	for _, opt := range options {
		if opt != nil {
			opt(o)
		}
	}
	if o.PidPath != "" {
		createPidFile(o.PidPath)
	}
	// start 钩子：zsf 框架（负序）与业务（正序）初始化完成后，配置等全局状态就绪
	for _, task := range start.GetInit() {
		task()
	}
	// 两类构造器都在此处才被调用（start 钩子已执行，配置等全局状态就绪），
	// 所以构造期读配置（端口、TLS 证书路径）用哪个都行；WithObjects 的构造器先跑。
	objects := make([]Object, 0)
	for _, builder := range o.Objects {
		objects = append(objects, builder())
	}
	for _, builder := range o.ObjectBuilders {
		objects = append(objects, builder()...)
	}
	if objects != nil {
		sort.SliceStable(objects, func(i, j int) bool {
			return objects[i].Order() < objects[j].Order()
		})
	}
	for _, l := range objects {
		l.OnApplicationStart()
	}
	quit.AddApplicationShutdownHook(func() {
		for _, object := range slices.Backward(objects) {
			object.OnApplicationShutdown()
		}
	})
	for _, l := range objects {
		l.AfterInitialize()
	}
	quit.Wait()
}

type option struct {
	PidPath        string
	Objects        []func() Object
	ObjectBuilders []func() []Object
}

// Option 是 lifecycle.Run 的配置项函数类型
type Option func(*option)

// WithPidFile 指定 PID 文件路径：Run 启动时创建目录并写入当前进程 PID，
// 便于外部编排（如 K8s liveness 探针）确认进程存活。
func WithPidFile(filePath string) Option {
	return func(opt *option) {
		opt.PidPath = filePath
	}
}

// WithObjects 设置生命周期对象（单个对象的构造器，可变参数）。
// 对象将按 Order() 升序启动。
//
// 覆盖语义：多次调用只有最后一次生效。装配必须集中在一处（main 里调用一次），
// 不允许各包的 init 各自追加——否则"进程由哪些对象组成"会散落在各处，读不出来。
func WithObjects(builders ...func() Object) Option {
	return func(opt *option) {
		opt.Objects = builders
	}
}

// WithObjectsList 设置一组生命周期对象（每个构造器返回一批对象，按注册顺序拼接）。
// 适合把同属一个服务的多个对象打包声明（如一个服务端、它的后台 worker）。
//
// 与 WithObjects 无"惰性/立即"之分：两类构造器都由 Run 在 start 钩子执行后调用，
// 构造期读静态配置（端口、TLS 证书路径）两种都可以用。
// 同样是覆盖语义：多次调用只有最后一次生效，装配集中在一处。
func WithObjectsList(builders ...func() []Object) Option {
	return func(opt *option) {
		opt.ObjectBuilders = builders
	}
}

func createPidFile(filePath string) {
	currentPid := os.Getpid()
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		logger.Logger.Fatal().Msgf("create PID folder: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(strconv.FormatInt(int64(currentPid), 10)), 0644); err != nil {
		logger.Logger.Fatal().Msgf("write PID information: %v", err)
	}
}
