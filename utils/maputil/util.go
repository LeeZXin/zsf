// Package maputil 提供泛型 map 工具：合并、取键与浅拷贝。
package maputil

import (
	"maps"
	"slices"
)

// MergeToLeft 将 right 的所有键值合并到 left（原地修改），相同 key 以 right 为准。
// 注意：left 为 nil 时写入会 panic，调用前需确保 left 已初始化。
func MergeToLeft[K comparable, V any](left map[K]V, right map[K]V) {
	for k, v := range right {
		left[k] = v
	}
}

// Keys 返回 map 的全部 key。
// 注意：顺序不保证（基于 map 迭代）；nil map 返回非 nil 空切片。
func Keys[K comparable, V any](m map[K]V) []K {
	return slices.Collect(maps.Keys(m))
}

// Copy 浅拷贝 map：返回的新 map 与原 map 不共享底层存储。
// 注意：值为引用类型时仅复制引用，不深拷贝。
func Copy[K comparable, V any](m map[K]V) map[K]V {
	ret := make(map[K]V, len(m))
	for k, v := range m {
		ret[k] = v
	}
	return ret
}
