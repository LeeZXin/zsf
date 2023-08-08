package cache

import (
	"context"
	"hash/crc32"
	"sync"
	"time"
)

// 分段segment 可过期map
// 默认分64个segment

const (
	segmentSize = 64
)

type segment[T any] struct {
	expireDuration time.Duration
	mu             sync.Mutex
	cache          map[string]*SingleCacheEntry[T]
	supplier       SupplierWithKey[T]
}

func newSegment[T any](supplier SupplierWithKey[T], expireDuration time.Duration) *segment[T] {
	return &segment[T]{
		mu:             sync.Mutex{},
		cache:          make(map[string]*SingleCacheEntry[T], 8),
		supplier:       supplier,
		expireDuration: expireDuration,
	}
}

func (e *segment[T]) getData(ctx context.Context, key string) (T, error) {
	getEntry := func() (*SingleCacheEntry[T], error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		entry, ok := e.cache[key]
		if ok {
			return entry, nil
		}
		entry, err := NewSingleCacheEntry(func(ctx context.Context) (T, error) {
			return e.supplier(ctx, key)
		}, e.expireDuration)
		if err != nil {
			return nil, err
		}
		e.cache[key] = entry
		return entry, nil
	}
	var ret T
	entry, err := getEntry()
	if err != nil {
		return ret, err
	}
	return entry.LoadData(ctx)
}

func (e *segment[T]) allKeys() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	keys := make([]string, 0, len(e.cache))
	for key := range e.cache {
		k := key
		keys = append(keys, k)
	}
	return keys
}

func (e *segment[T]) clear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for key := range e.cache {
		delete(e.cache, key)
	}
}

func (e *segment[T]) removeKey(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.cache, key)
}

func (e *segment[T]) containsKey(key string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.cache[key]
	return ok
}

type LocalCache[T any] struct {
	supplier SupplierWithKey[T]
	segments []*segment[T]
}

func NewLocalCache[T any](supplier SupplierWithKey[T], duration time.Duration) (ExpireCache[T], error) {
	if supplier == nil {
		return nil, NilSupplierErr
	}
	if duration <= 0 {
		return nil, IllegalDurationErr
	}
	segments := make([]*segment[T], 0, segmentSize)
	for i := 0; i < segmentSize; i++ {
		segments = append(segments, newSegment(supplier, duration))
	}
	return &LocalCache[T]{
		segments: segments,
		supplier: supplier,
	}, nil
}

func (e *LocalCache[T]) LoadData(ctx context.Context, key string) (T, error) {
	return e.getSegment(key).getData(ctx, key)
}

func (e *LocalCache[T]) getSegment(key string) *segment[T] {
	// mod 64
	index := hash(key) & 0x3f
	return e.segments[index]
}

func (e *LocalCache[T]) RemoveKey(key string) {
	e.getSegment(key).removeKey(key)
}

func (e *LocalCache[T]) AllKeys() []string {
	ret := make([]string, 0)
	for _, seg := range e.segments {
		ret = append(ret, seg.allKeys()...)
	}
	return ret
}

func (e *LocalCache[T]) Clear() {
	for _, seg := range e.segments {
		seg.clear()
	}
}

func (e *LocalCache[T]) ContainsKey(key string) bool {
	return e.getSegment(key).containsKey(key)
}

func hash(key string) int {
	ret := crc32.ChecksumIEEE([]byte(key))
	return int(ret)
}
