package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// SingleCache 单个数据缓存
// 带过期时间

type SingleCacheEntry struct {
	expireDuration time.Duration
	expireTime     atomic.Value
	data           atomic.Value
	mu             sync.Mutex
	supplier       Supplier
}

func NewSingleCacheEntry(supplier Supplier, duration time.Duration) (*SingleCacheEntry, error) {
	if supplier == nil {
		return nil, NilSupplierErr
	}
	if duration <= 0 {
		return nil, IllegalDurationErr
	}
	return &SingleCacheEntry{
		expireDuration: duration,
		expireTime:     atomic.Value{},
		data:           atomic.Value{},
		mu:             sync.Mutex{},
		supplier:       supplier,
	}, nil
}

func (e *SingleCacheEntry) LoadData(ctx context.Context) (any, error) {
	var (
		result any
		err    error
	)
	etime := e.expireTime.Load()
	// 首次加载
	if etime == nil {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.expireTime.Load() != nil {
			return e.data.Load(), nil
		}
		result, err = e.supplier(ctx)
		if err != nil {
			return nil, err
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
	return e.data.Load(), nil
}
