package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// SingleCache 单个数据缓存
// 带过期时间

type SingleCacheEntry[T any] struct {
	expireDuration time.Duration
	expireTime     atomic.Value
	data           atomic.Value
	mu             sync.Mutex
	supplier       Supplier[T]
}

func NewSingleCacheEntry[T any](supplier Supplier[T], duration time.Duration) (*SingleCacheEntry[T], error) {
	if supplier == nil {
		return nil, NilSupplierErr
	}
	if duration <= 0 {
		return nil, IllegalDurationErr
	}
	return &SingleCacheEntry[T]{
		expireDuration: duration,
		expireTime:     atomic.Value{},
		data:           atomic.Value{},
		mu:             sync.Mutex{},
		supplier:       supplier,
	}, nil
}

func (e *SingleCacheEntry[T]) LoadData(ctx context.Context) (T, error) {
	var (
		result T
		err    error
	)
	etime := e.expireTime.Load()
	// 首次加载
	if etime == nil {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.expireTime.Load() != nil {
			return e.data.Load().(T), nil
		}
		result, err = e.supplier(ctx)
		if err != nil {
			return result, err
		}
		e.data.Store(result)
		e.expireTime.Store(time.Now().Add(e.expireDuration))
		return result, nil
	}
	now := time.Now()
	if etime.(time.Time).Before(now) {
		// 过期
		if e.mu.TryLock() {
			defer e.mu.Unlock()
			result, err = e.supplier(ctx)
			if err == nil {
				e.data.Store(result)
				e.expireTime.Store(time.Now().Add(e.expireDuration))
			}
			return result, nil
		}
	}
	return e.data.Load().(T), nil
}
