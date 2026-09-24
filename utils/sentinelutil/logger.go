package sentinelutil

type slogger struct {
}

func (*slogger) Debug(string, ...any) {}

func (*slogger) DebugEnabled() bool {
	return false
}

func (*slogger) Info(string, ...any) {}

func (*slogger) InfoEnabled() bool {
	return false
}

func (*slogger) Warn(string, ...any) {}

func (*slogger) WarnEnabled() bool {
	return false
}

func (*slogger) Error(error, string, ...any) {}

func (*slogger) ErrorEnabled() bool {
	return true
}
