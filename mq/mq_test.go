package mq

import (
	"flag"
	"fmt"
	"github.com/LeeZXin/zsf/util/threadutil"
	"testing"
	"time"
)

func BenchmarkNewKafkaConsumer(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = threadutil.RunSafe(func() {
			var err error
			fmt.Println(err.Error())
		})
	}
}

func TestNewKafkaConsumer(t *testing.T) {
	flag.Parse()
	milli := time.UnixMilli(time.Now().UnixMilli())
	fmt.Println(milli)
}
