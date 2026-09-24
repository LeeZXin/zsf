// Package listutil 提供泛型切片操作与 eager 链式管道 Pipe，供业务侧组合过滤、映射、去重、展平与收口。
// 所有函数与 Pipe 中间操作均不修改入参切片。
package listutil

// Contains 返回 arr 中是否存在满足 fn 的元素（fn 返回 true）。
// fn 返回 error 时立即终止并返回该错误。
func Contains[T any](arr []T, fn func(T) (bool, error)) (bool, error) {
	_, b, err := FindFirst(arr, fn)
	return b, err
}

// ContainsNe 是 Contains 的无 error 版本，fn 返回 true 即视为命中。
func ContainsNe[T any](arr []T, fn func(T) bool) bool {
	_, b := FindFirstNe(arr, fn)
	return b
}

// All 返回 arr 是否全部满足 fn（空切片为 true）。
// fn 返回 error 时立即终止并返回该错误。
func All[T any](arr []T, fn func(T) (bool, error)) (bool, error) {
	for _, t := range arr {
		b, err := fn(t)
		if err != nil {
			return false, err
		}
		if !b {
			return false, nil
		}
	}
	return true, nil
}

// AllNe 是 All 的无 error 版本。
func AllNe[T any](arr []T, fn func(T) bool) bool {
	for _, t := range arr {
		if !fn(t) {
			return false
		}
	}
	return true
}

// FindFirst 返回 arr 中第一个满足 fn 的元素及命中标记。
// fn 返回 error 时立即终止并返回该错误；未命中返回零值 T 和 false。
func FindFirst[T any](arr []T, fn func(T) (bool, error)) (T, bool, error) {
	for _, t := range arr {
		b, err := fn(t)
		if err != nil {
			return t, false, err
		}
		if b {
			return t, true, nil
		}
	}
	var t T
	return t, false, nil
}

// FindFirstNe 是 FindFirst 的无 error 版本；未命中返回零值 T 和 false。
func FindFirstNe[T any](arr []T, fn func(T) bool) (T, bool) {
	for _, t := range arr {
		if fn(t) {
			return t, true
		}
	}
	var t T
	return t, false
}

// Filter 过滤出满足 fn 的元素，保持原有顺序。
// fn 返回 error 时立即终止并返回 nil（而非空切片）。
func Filter[T any](data []T, fn func(T) (bool, error)) ([]T, error) {
	ret := make([]T, 0)
	for _, d := range data {
		b, err := fn(d)
		if err != nil {
			return nil, err
		}
		if b {
			ret = append(ret, d)
		}
	}
	return ret, nil
}

// FilterNe 是 Filter 的无 error 版本；输入为空时返回非 nil 空切片。
func FilterNe[T any](data []T, fn func(T) bool) []T {
	ret := make([]T, 0)
	for _, d := range data {
		if fn(d) {
			ret = append(ret, d)
		}
	}
	return ret
}

// Map 将每个元素经 mapper 映射为新类型，保持原有顺序。
// mapper 返回 error 时立即终止并返回 nil（而非空切片）。
func Map[T, K any](data []T, mapper func(T) (K, error)) ([]K, error) {
	ret := make([]K, 0, len(data))
	for _, d := range data {
		k, err := mapper(d)
		if err != nil {
			return nil, err
		}
		ret = append(ret, k)
	}
	return ret, nil
}

// MapNe 是 Map 的无 error 版本，mapper 不返回错误。
func MapNe[T, K any](data []T, mapper func(T) K) []K {
	ret := make([]K, 0, len(data))
	for _, d := range data {
		ret = append(ret, mapper(d))
	}
	return ret
}

// MapWithIndex 类似 Map，但 mapper 额外接收元素下标（从 0 开始）。
// mapper 返回 error 时立即终止并返回 nil。
func MapWithIndex[T, K any](data []T, mapper func(T, int) (K, error)) ([]K, error) {
	ret := make([]K, 0, len(data))
	for i, d := range data {
		k, err := mapper(d, i)
		if err != nil {
			return nil, err
		}
		ret = append(ret, k)
	}
	return ret, nil
}

// MapNeWithIndex 是 MapWithIndex 的无 error 版本，mapper 不返回错误。
func MapNeWithIndex[T, K any](data []T, mapper func(T, int) K) []K {
	ret := make([]K, 0, len(data))
	for i, d := range data {
		ret = append(ret, mapper(d, i))
	}
	return ret
}

// FlatMap 将每个元素映射为切片后再展平为一维，保持原有顺序。
// mapper 返回 error 时立即终止并返回 nil。
func FlatMap[T, K any](data []T, mapper func(T) ([]K, error)) ([]K, error) {
	ret := make([]K, 0)
	for _, d := range data {
		ks, err := mapper(d)
		if err != nil {
			return nil, err
		}
		if len(ks) > 0 {
			ret = append(ret, ks...)
		}
	}
	return ret, nil
}

// FlatMapNe 是 FlatMap 的无 error 版本。
func FlatMapNe[T, K any](data []T, mapper func(T) []K) []K {
	ret := make([]K, 0)
	for _, d := range data {
		ret = append(ret, mapper(d)...)
	}
	return ret
}

// Distinct 去重：保留每个元素首次出现的位置，并保持原有顺序。
// 参数为空时返回非 nil 空切片。
func Distinct[T comparable](data ...T) []T {
	seen := make(map[T]struct{}, len(data))
	result := make([]T, 0, len(data))
	for _, t := range data {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		result = append(result, t)
	}
	return result
}

// DistinctBy 按 uniqKey 去重：保留每个 key 首次出现的元素，并保持原有顺序。
// uniqKey 返回 error 时立即终止并返回 nil。
func DistinctBy[T any, K comparable](data []T, uniqKey func(T) (K, error)) ([]T, error) {
	seen := make(map[K]struct{}, len(data))
	result := make([]T, 0, len(data))
	for _, t := range data {
		key, err := uniqKey(t)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, t)
	}
	return result, nil
}

// DistinctByNe 是 DistinctBy 的无 error 版本。
func DistinctByNe[T any, K comparable](data []T, uniqKey func(T) K) []T {
	seen := make(map[K]struct{}, len(data))
	result := make([]T, 0, len(data))
	for _, t := range data {
		key := uniqKey(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, t)
	}
	return result
}

// FlatNe 将二维切片展平为一维，保持原有顺序。
// 输入为 nil 或空时返回非 nil 空切片。
func FlatNe[T any](data [][]T) []T {
	length := 0
	for _, d := range data {
		length += len(d)
	}
	ret := make([]T, 0, length)
	for _, d := range data {
		if len(d) > 0 {
			ret = append(ret, d...)
		}
	}
	return ret
}

// ToMapNe 将切片转换为 map：f 返回每个元素的 key 与 value。
// 注意：相同 key 时后出现的元素覆盖先出现的元素。
func ToMapNe[T any, K comparable, V any](t []T, f func(T) (K, V)) map[K]V {
	ret := make(map[K]V, len(t))
	for _, item := range t {
		k, v := f(item)
		ret[k] = v
	}
	return ret
}

// ToMapListNe 将切片按 key 分组：相同 key 的值聚合成切片。
// 同一 key 的元素在结果切片中保持原顺序。
func ToMapListNe[T any, K comparable, V any](t []T, f func(T) (K, V)) map[K][]V {
	ret := make(map[K][]V, len(t))
	for _, item := range t {
		k, v := f(item)
		ret[k] = append(ret[k], v)
	}
	return ret
}

// ConcatAll 按顺序拼接多个切片为一个新切片。
// 注意：不修改入参切片；入参为空时返回非 nil 空切片。
func ConcatAll[T any](data ...[]T) []T {
	length := 0
	for _, d := range data {
		length += len(d)
	}
	ret := make([]T, 0, length)
	for _, items := range data {
		ret = append(ret, items...)
	}
	return ret
}
