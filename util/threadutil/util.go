package threadutil

import (
	"errors"
	"github.com/LeeZXin/zsf/util/runtimeutil"
)

func RunSafe(fn func(), cleanup ...func()) error {
	echan := make(chan error, 1)
	defer close(echan)
	runSafe(fn, echan, cleanup...)
	select {
	case err := <-echan:
		return err
	default:
		return nil
	}
}

func runSafe(fn func(), echan chan error, cleanup ...func()) {
	defer doRecover(echan, cleanup...)
	fn()
}

func doRecover(echan chan error, after ...func()) {
	for _, fn := range after {
		fn()
	}
	if e := recover(); e != nil {
		err, ok := e.(error)
		if !ok {
			err = errors.New("unknown errors")
		}
		echan <- runtimeutil.NewRuntimeErr(err, 10, 6)
	}
}
