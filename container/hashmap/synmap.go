package hashmap

import "sync"

type SyncMap[K comparable, V any] struct {
	m sync.Map
}

func NewSyncMap[K comparable, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		m: sync.Map{},
	}
}

func (s *SyncMap[K, V]) Load(key K) (V, bool) {
	v, b := s.m.Load(key)
	if b {
		return v.(V), true
	}
	var vv V
	return vv, false
}

func (s *SyncMap[K, V]) Store(key K, v V) {
	s.m.Store(key, v)
}

func (s *SyncMap[K, V]) Delete(key K) {
	s.m.Delete(key)
}

func (s *SyncMap[K, V]) Range(f func(key K, value V) bool) {
	s.m.Range(func(key, value any) bool {
		return f(key.(K), value.(V))
	})
}

func (s *SyncMap[K, V]) Clear() {
	s.m.Clear()
}

func (s *SyncMap[K, V]) LoadOrStore(key K, value V) (V, bool) {
	v, b := s.m.LoadOrStore(key, value)
	return v.(V), b
}

func (s *SyncMap[K, V]) LoadAndDelete(key K) (V, bool) {
	v, b := s.m.LoadAndDelete(key)
	return v.(V), b
}

func (s *SyncMap[K, V]) Swap(key K, value V) (V, bool) {
	prev, loaded := s.m.Swap(key, value)
	if loaded {
		return prev.(V), true
	}
	var zero V
	return zero, false
}

func (s *SyncMap[K, V]) CompareAndSwap(key K, oldValue V, newValue V) bool {
	return s.m.CompareAndSwap(key, oldValue, newValue)
}

func (s *SyncMap[K, V]) CompareAndDelete(key K, value V) bool {
	return s.m.CompareAndDelete(key, value)
}

func NewStringSyncMap[V any]() *SyncMap[string, V] {
	return NewSyncMap[string, V]()
}
