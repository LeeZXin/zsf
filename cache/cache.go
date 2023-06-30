package cache

import (
	"context"
	"errors"
)

var (
	NilSupplierErr     = errors.New("nil supplier")
	IllegalDurationErr = errors.New("illegal duration")
	IllegalMaxSizeErr  = errors.New("maxSize should greater than 0")
)

type Supplier func(context.Context) (any, error)

type SupplierWithKey func(context.Context, string) (any, error)

type ExpireCache interface {
	// LoadData 获取数据
	LoadData(ctx context.Context, key string) (any, error)
	// RemoveKey 删除key
	RemoveKey(key string)
	// AllKeys 获取所有的key
	AllKeys() []string
	// Clear 清除
	Clear()
	// ContainsKey 包含某个key
	ContainsKey(key string) bool
}
