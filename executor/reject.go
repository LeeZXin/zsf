package executor

import "errors"

//拒绝策略
//默认实现两种
// AbortPolicy 直接丢弃
// CallerRunsPolicy 由主协程执行

type RejectHandler interface {
	RejectedExecution(runnable Runnable, executor *Executor) error
}

type AbortPolicy struct{}

func (*AbortPolicy) RejectedExecution(runnable Runnable, executor *Executor) error {
	return errors.New("task rejected by executor")
}

type CallerRunsPolicy struct{}

func (*CallerRunsPolicy) RejectedExecution(runnable Runnable, executor *Executor) error {
	runnable.Run()
	return nil
}
