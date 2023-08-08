package cache

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestNewLocalCache(t *testing.T) {
	entry, err := NewSingleCacheEntry[string](func(ctx context.Context) (string, error) {
		return "ggg", nil
	}, 10*time.Second)
	if err != nil {
		panic(err)
	}
	for i := 0; i < 1000; i++ {
		fmt.Println(entry.LoadData(context.Background()))
	}
}
