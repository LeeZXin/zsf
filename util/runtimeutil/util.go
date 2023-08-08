package runtimeutil

import (
	"runtime"
	"strconv"
	"strings"
)

func PrettyErrCallerTrace(depth int, starts ...int) string {
	stack := make([]string, 0, depth)
	start := 0
	if starts != nil && len(starts) > 0 {
		start = starts[0]
	}
	for i := start; i < depth+start; i++ {
		_, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		stack = append(stack, "		"+file+":"+strconv.Itoa(line))
	}
	return strings.Join(stack, "\n")
}

type RuntimeErr struct {
	err        error
	callTraces string
}

func (r *RuntimeErr) Error() string {
	if r.err == nil {
		return ""
	}
	return r.err.Error() + "\n" + r.callTraces
}

func NewRuntimeErr(err error, depth int, skip ...int) error {
	return &RuntimeErr{
		err:        err,
		callTraces: PrettyErrCallerTrace(depth, skip...),
	}
}
