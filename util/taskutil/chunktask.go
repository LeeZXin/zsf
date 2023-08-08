package taskutil

import (
	"errors"
	"github.com/LeeZXin/zsf/util/threadutil"
	"sync"
	"time"
)

type Chunk[T any] struct {
	Size int
	Data T
}

type FlushFn[T any] func([]Chunk[T])

type ChunkTask[T any] struct {
	mu sync.Mutex

	triggerSize int
	dataSize    int
	chunkList   []Chunk[T]

	fn FlushFn[T]
	pt *PeriodicalTask
}

func NewChunkTask[T any](triggerSize int, fn FlushFn[T], flushInterval time.Duration) (*ChunkTask[T], error) {
	if fn == nil || flushInterval == 0 {
		return nil, errors.New("invalid task arguments")
	}
	ret := &ChunkTask[T]{
		mu:          sync.Mutex{},
		triggerSize: triggerSize,
		dataSize:    0,
		chunkList:   make([]Chunk[T], 0, 8),
		fn:          fn,
	}
	pt, _ := NewPeriodicalTask(flushInterval, ret.Flush)
	ret.pt = pt
	return ret, nil
}

func (t *ChunkTask[T]) Start() {
	t.pt.Start()
}

func (t *ChunkTask[T]) Stop() {
	t.pt.Stop()
}

func (t *ChunkTask[T]) Execute(data T, dataSize int) {
	if t.add(data, dataSize) {
		t.Flush()
	}
}

func (t *ChunkTask[T]) Flush() {
	t.mu.Lock()
	defer t.mu.Unlock()
	threadutil.RunSafe(func() {
		t.fn(t.chunkList)
	})
	t.chunkList = make([]Chunk[T], 0, 8)
	t.dataSize = 0
}

func (t *ChunkTask[T]) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.chunkList = make([]Chunk[T], 0, 8)
	t.dataSize = 0
}

func (t *ChunkTask[T]) add(data T, size int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.chunkList = append(t.chunkList, Chunk[T]{
		Size: size,
		Data: data,
	})
	t.dataSize += size
	return t.dataSize >= t.triggerSize
}
