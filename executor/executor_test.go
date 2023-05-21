package executor

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestFuture(t *testing.T) {
	e, _ := NewExecutor(10, 1024, 10*time.Minute, &CallerRunsPolicy{})
	futureTask, err := e.Submit(func() (any, error) {
		time.Sleep(10 * time.Second)
		return "ss", errors.New("something wrong")
	})
	if err != nil {
		panic(err)
	}
	result, err := futureTask.GetWithTimeout(4 * time.Second)
	fmt.Println(result, err)
}

func TestFuturePromise(t *testing.T) {
	e, _ := NewExecutor(10, 1024, 10*time.Minute, &CallerRunsPolicy{})
	futureTask, err := e.Submit(func() (any, error) {
		time.Sleep(10 * time.Second)
		return "ss", errors.New("something wrong")
	})
	if err != nil {
		panic(err)
	}
	go func() {
		time.Sleep(10 * time.Second)
		fmt.Println(futureTask.SetError(errors.New("wrong ! ooops")))
	}()
	result, err := futureTask.Get()
	fmt.Println(result, err)
}

func TestShutdown(t *testing.T) {
	e, _ := NewExecutor(10, 1024, 10*time.Minute, &CallerRunsPolicy{})
	e.Shutdown()
	for i := 0; i < 10; i++ {
		go func() {
			err := e.Execute(func() {
				time.Sleep(10 * time.Second)
				fmt.Println("do something")
			})
			if err != nil {
				fmt.Println(err)
			}
		}()
	}
	select {}
}
