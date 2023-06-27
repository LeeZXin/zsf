package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	NilSupplierError   = errors.New("nil supplier")
	IllegalDuration    = errors.New("illegal duration")
	IllegalSegmentSize = errors.New("illegal segment size")
)

type Supplier func(context.Context) (any, error)

type SingleCacheEntry struct {
	expireDuration time.Duration
	expireTime     atomic.Value
	data           atomic.Value
	mu             sync.Mutex
	supplier       Supplier
}

func NewSingleCacheEntry(supplier Supplier, duration time.Duration) (*SingleCacheEntry, error) {
	if supplier == nil {
		return nil, NilSupplierError
	}
	if duration <= 0 {
		return nil, IllegalDuration
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
