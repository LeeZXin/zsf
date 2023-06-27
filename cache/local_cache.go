package cache

import (
	"context"
	"hash/crc32"
	"sync"
	"time"
)

const (
	segmentSize = 64
)

type SupplierWithKey func(context.Context, string) (any, error)

type MapCache map[string]*SingleCacheEntry

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

func (e *segment) LoadData(ctx context.Context, key string) (any, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	entry, ok := e.cache[key]
	if ok {
		return entry.LoadData(ctx)
	}
	entry, err := NewSingleCacheEntry(func(ctx context.Context) (any, error) {
		return e.supplier(ctx, key)
	}, e.expireDuration)
	if err != nil {
		return nil, err
	}
	e.cache[key] = entry
	return entry.LoadData(ctx)
}

func (e *segment) AllKeys() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	keys := make([]string, 0, len(e.cache))
	for key := range e.cache {
		k := key
		keys = append(keys, k)
	}
	return keys
}

func (e *segment) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for key := range e.cache {
		delete(e.cache, key)
	}
}

func (e *segment) RemoveKey(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.cache, key)
}

type LocalCache struct {
	supplier SupplierWithKey
	segments []*segment
}

func NewLocalCache(supplier SupplierWithKey, duration time.Duration) (*LocalCache, error) {
	if supplier == nil {
		return nil, NilSupplierError
	}
	if duration <= 0 {
		return nil, IllegalDuration
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
	return e.getSegment(key).LoadData(ctx, key)
}

func (e *LocalCache) getSegment(key string) *segment {
	// mod 64
	index := hash(key) & 0x3f
	return e.segments[index]
}

func (e *LocalCache) RemoveKey(key string) {
	e.getSegment(key).RemoveKey(key)
}

func (e *LocalCache) AllKeys() []string {
	ret := make([]string, 0)
	for _, seg := range e.segments {
		ret = append(ret, seg.AllKeys()...)
	}
	return ret
}

func (e *LocalCache) Clear() {
	for _, seg := range e.segments {
		seg.Clear()
	}
}

func hash(key string) int {
	ret := crc32.ChecksumIEEE([]byte(key))
	return int(ret)
}
