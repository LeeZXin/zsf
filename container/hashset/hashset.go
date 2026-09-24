// Package hashset 提供基于 map 的泛型集合（Hashset），支持增删查、清空与
// 克隆等基础操作。
//
// 注意：
//   - 底层为 map，非并发安全，多 goroutine 并发读写需由调用方自行加锁
//   - All 返回的元素顺序随机（Go map 迭代顺序不定），不要依赖其顺序
package hashset

// Hashset 泛型集合，底层为 map[T]struct{}，元素自动去重。
// Add/Remove/Clear 均为原地修改并返回自身，支持链式调用。
type Hashset[T comparable] map[T]struct{}

// NewHashset 创建包含 ts 中全部元素的集合（重复元素自动去重）。
func NewHashset[T comparable](ts ...T) Hashset[T] {
	res := make(map[T]struct{}, len(ts))
	for _, t := range ts {
		res[t] = struct{}{}
	}
	return res
}

// Len 返回集合中的元素个数。
func (s Hashset[T]) Len() int {
	return len(s)
}

// All 返回集合全部元素组成的切片（新切片，可安全修改）；
// 元素顺序随机（map 迭代顺序不定），不要依赖。
func (s Hashset[T]) All() []T {
	res := make([]T, 0, s.Len())
	for t := range s {
		res = append(res, t)
	}
	return res
}

// Contains 判断元素 t 是否在集合中。
func (s Hashset[T]) Contains(t T) bool {
	_, ok := s[t]
	return ok
}

// Remove 从集合中移除指定元素（不存在的元素静默忽略），返回自身。
func (s Hashset[T]) Remove(ts ...T) Hashset[T] {
	for _, t := range ts {
		delete(s, t)
	}
	return s
}

// Clear 清空集合，返回自身。
func (s Hashset[T]) Clear() Hashset[T] {
	clear(s)
	return s
}

// Add 向集合添加元素（已存在则忽略），返回自身。
func (s Hashset[T]) Add(ts ...T) Hashset[T] {
	for _, t := range ts {
		s[t] = struct{}{}
	}
	return s
}

// Clone 深拷贝当前集合，返回独立的新集合，与源集合互不影响。
func (s Hashset[T]) Clone() Hashset[T] {
	return NewHashset(s.All()...)
}
