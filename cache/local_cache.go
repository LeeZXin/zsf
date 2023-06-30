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

type segment struct {
	expireDuration time.Duration
	mu             sync.Mutex
	cache          map[string]*SingleCacheEntry
	supplier       SupplierWithKey
}

func newSegment(supplier SupplierWithKey, expireDuration time.Duration) *segment {
	return &segment{
		mu:             sync.Mutex{},
		cache:          make(map[string]*SingleCacheEntry, 8),
		supplier:       supplier,
		expireDuration: expireDuration,
	}
}

func (e *segment) getData(ctx context.Context, key string) (any, error) {
	getEntry := func() (*SingleCacheEntry, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		entry, ok := e.cache[key]
		if ok {
			return entry, nil
		}
		entry, err := NewSingleCacheEntry(func(ctx context.Context) (any, error) {
			return e.supplier(ctx, key)
		}, e.expireDuration)
		if err != nil {
			return nil, err
		}
		e.cache[key] = entry
		return entry, nil
	}
	entry, err := getEntry()
	if err != nil {
		return nil, err
	}
	return entry.LoadData(ctx)
}

func (e *segment) allKeys() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	keys := make([]string, 0, len(e.cache))
	for key := range e.cache {
		k := key
		keys = append(keys, k)
	}
	return keys
}

func (e *segment) clear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for key := range e.cache {
		delete(e.cache, key)
	}
}

func (e *segment) removeKey(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.cache, key)
}

func (e *segment) containsKey(key string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.cache[key]
	return ok
}

type LocalCache struct {
	supplier SupplierWithKey
	segments []*segment
}

func NewLocalCache(supplier SupplierWithKey, duration time.Duration) (*LocalCache, error) {
	if supplier == nil {
		return nil, NilSupplierErr
	}
	if duration <= 0 {
		return nil, IllegalDurationErr
	}
	segments := make([]*segment, 0, segmentSize)
	for i := 0; i < segmentSize; i++ {
		segments = append(segments, newSegment(supplier, duration))
	}
	return &LocalCache{
		segments: segments,
		supplier: supplier,
	}, nil
}

func (e *LocalCache) LoadData(ctx context.Context, key string) (any, error) {
	return e.getSegment(key).getData(ctx, key)
}

func (e *LocalCache) getSegment(key string) *segment {
	// mod 64
	index := hash(key) & 0x3f
	return e.segments[index]
}

func (e *LocalCache) RemoveKey(key string) {
	e.getSegment(key).removeKey(key)
}

func (e *LocalCache) AllKeys() []string {
	ret := make([]string, 0)
	for _, seg := range e.segments {
		ret = append(ret, seg.allKeys()...)
	}
	return ret
}

func (e *LocalCache) Clear() {
	for _, seg := range e.segments {
		seg.clear()
	}
}

func (e *LocalCache) ContainsKey(key string) bool {
	return e.getSegment(key).containsKey(key)
}

func hash(key string) int {
	ret := crc32.ChecksumIEEE([]byte(key))
	return int(ret)
}
