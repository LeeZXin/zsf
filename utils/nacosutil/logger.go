package nacosutil

import (
	"fmt"

	"github.com/LeeZXin/zsf/logger"
)

// nlogger 把 Nacos SDK 的日志转发到 zsf logger。
//
// 级别映射有意收窄：SDK 的 Info 里有逐请求日志（SelectInstances 每次调用都打一行
// "select instances with options ..."，数据其实来自本地缓存），转发路径上会把日志刷满，
// 因此 Info/Debug 直接丢弃；Warn/Error 保留——SDK 的 Warn 是建连与重连失败、重新订阅/
// 重新注册失败、配置监听失败这类低频故障，而转发失败对外只表现为 404，日志是唯一线索。
//
// SDK 的 constant.WithLogLevel 在这里不起作用：SetLogger 之后 SDK 的
// logger.InitLogger 因 logger != nil 直接返回，不会再装回自己的 zap logger，
// 级别只能由本 adapter 决定。
type nlogger struct {
}

func (*nlogger) Info(...any) {}

func (*nlogger) Warn(args ...any) {
	logger.Logger.Warn().Msg(fmt.Sprint(args...))
}

func (*nlogger) Error(args ...any) {
	logger.Logger.Error().Msg(fmt.Sprint(args...))
}

func (*nlogger) Debug(...any) {}

func (*nlogger) Infof(string, ...any) {}

func (*nlogger) Warnf(format string, args ...any) {
	logger.Logger.Warn().Msgf(format, args...)
}

func (*nlogger) Errorf(format string, args ...any) {
	logger.Logger.Error().Msgf(format, args...)
}

func (*nlogger) Debugf(string, ...any) {}

func (*nlogger) Close() error {
	return nil
}
