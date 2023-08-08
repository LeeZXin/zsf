package taskutil

import (
	"fmt"
	"testing"
	"time"
)

func TestNewPeriodicalTask(t *testing.T) {
	task, err := NewChunkTask[string](100, func(data []Chunk[string]) {
		fmt.Println(data)
		fmt.Println("flush")
	}, 10*time.Second)
	if err != nil {
		panic(err)
	}
	task.Start()
	defer task.Stop()
	for i := 0; i < 2; i++ {
		task.Execute("xxx", 10)
	}
	go func() {
		time.Sleep(12 * time.Second)
		for i := 0; i < 10; i++ {
			task.Execute("101", 101)
		}
		task.Stop()
	}()
	select {}
}
