package listutil

import "slices"

// Pipe 是切片的 eager 链式管道，按调用顺序立即求值。
//
// 同类型中间操作（Filter / DistinctBy / Concat / Limit / Skip / SortFunc）原地更新接收者并返回自身；
// Map / FlatMap 因元素类型变化返回新管道。同一 Pipe 不要分叉复用。
// 入参切片不会被修改。链路中一旦出错，后续中间操作不再执行，终端操作返回该错误。
type Pipe[T any] struct {
	err  error
	data []T
}

// NewPipe 用 data 构造管道。不拷贝切片；在发生会分配新切片的中间操作之前，Data 与入参共享底层数组。
func NewPipe[T any](data []T) *Pipe[T] {
	return &Pipe[T]{data: data}
}

// Err 返回链路中第一个错误；未出错时为 nil。
func (p *Pipe[T]) Err() error {
	return p.err
}

// Data 返回当前切片。若链路中已出错，返回 (nil, err)。
func (p *Pipe[T]) Data() ([]T, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.data, nil
}

// Len 返回当前元素个数。若链路中已出错，返回 (0, err)。
func (p *Pipe[T]) Len() (int, error) {
	if p.err != nil {
		return 0, p.err
	}
	return len(p.data), nil
}

// First 返回第一个元素。空切片返回零值与 false；链路出错时返回该错误。
func (p *Pipe[T]) First() (T, bool, error) {
	if p.err != nil {
		var z T
		return z, false, p.err
	}
	if len(p.data) == 0 {
		var z T
		return z, false, nil
	}
	return p.data[0], true, nil
}

// Last 返回最后一个元素。空切片返回零值与 false；链路出错时返回该错误。
func (p *Pipe[T]) Last() (T, bool, error) {
	if p.err != nil {
		var z T
		return z, false, p.err
	}
	if len(p.data) == 0 {
		var z T
		return z, false, nil
	}
	return p.data[len(p.data)-1], true, nil
}

// FilterNe 原地过滤出满足 fn 的元素，返回 p 自身。已出错时跳过。
func (p *Pipe[T]) FilterNe(fn func(T) bool) *Pipe[T] {
	if p.err == nil {
		p.data = FilterNe(p.data, fn)
	}
	return p
}

// Filter 原地过滤出满足 fn 的元素，返回 p 自身。fn 出错或链路已出错时后续步骤短路。
func (p *Pipe[T]) Filter(fn func(T) (bool, error)) *Pipe[T] {
	if p.err == nil {
		p.data, p.err = Filter(p.data, fn)
	}
	return p
}

// MapNe 将元素映射为新类型并返回新管道；已出错时原样传递错误。
func (p *Pipe[T]) MapNe[K any](fn func(T) K) *Pipe[K] {
	if p.err != nil {
		return &Pipe[K]{err: p.err}
	}
	return &Pipe[K]{data: MapNe(p.data, fn)}
}

// Map 将元素映射为新类型并返回新管道；mapper 出错或链路已出错时后续步骤短路。
func (p *Pipe[T]) Map[K any](fn func(T) (K, error)) *Pipe[K] {
	if p.err != nil {
		return &Pipe[K]{err: p.err}
	}
	data, err := Map(p.data, fn)
	return &Pipe[K]{data: data, err: err}
}

// MapNeWithIndex 类似 MapNe，mapper 额外接收元素下标（从 0 开始）。
func (p *Pipe[T]) MapNeWithIndex[K any](fn func(T, int) K) *Pipe[K] {
	if p.err != nil {
		return &Pipe[K]{err: p.err}
	}
	return &Pipe[K]{data: MapNeWithIndex(p.data, fn)}
}

// MapWithIndex 类似 Map，mapper 额外接收元素下标（从 0 开始）。
func (p *Pipe[T]) MapWithIndex[K any](fn func(T, int) (K, error)) *Pipe[K] {
	if p.err != nil {
		return &Pipe[K]{err: p.err}
	}
	data, err := MapWithIndex(p.data, fn)
	return &Pipe[K]{data: data, err: err}
}

// FlatMapNe 将每个元素映射为切片后展平，返回新管道；已出错时原样传递错误。
func (p *Pipe[T]) FlatMapNe[K any](fn func(T) []K) *Pipe[K] {
	if p.err != nil {
		return &Pipe[K]{err: p.err}
	}
	return &Pipe[K]{data: FlatMapNe(p.data, fn)}
}

// FlatMap 将每个元素映射为切片后展平，返回新管道；mapper 出错或链路已出错时后续步骤短路。
func (p *Pipe[T]) FlatMap[K any](fn func(T) ([]K, error)) *Pipe[K] {
	if p.err != nil {
		return &Pipe[K]{err: p.err}
	}
	data, err := FlatMap(p.data, fn)
	return &Pipe[K]{data: data, err: err}
}

// DistinctByNe 按 uniqKey 原地去重（保留首次出现），返回 p 自身。已出错时跳过。
func (p *Pipe[T]) DistinctByNe[K comparable](uniqKey func(T) K) *Pipe[T] {
	if p.err == nil {
		p.data = DistinctByNe(p.data, uniqKey)
	}
	return p
}

// DistinctBy 按 uniqKey 原地去重（保留首次出现），返回 p 自身。uniqKey 出错或链路已出错时后续步骤短路。
func (p *Pipe[T]) DistinctBy[K comparable](uniqKey func(T) (K, error)) *Pipe[T] {
	if p.err == nil {
		p.data, p.err = DistinctBy(p.data, uniqKey)
	}
	return p
}

// Concat 按顺序追加若干切片，返回 p 自身。已出错时跳过。
func (p *Pipe[T]) Concat(more ...[]T) *Pipe[T] {
	if p.err == nil {
		p.data = ConcatAll(append([][]T{p.data}, more...)...)
	}
	return p
}

// Limit 只保留前 n 个元素，返回 p 自身。n <= 0 得到空切片；n 大于长度则不变。已出错时跳过。
func (p *Pipe[T]) Limit(n int) *Pipe[T] {
	if p.err != nil {
		return p
	}
	if n <= 0 {
		p.data = make([]T, 0)
		return p
	}
	if n < len(p.data) {
		p.data = slices.Clone(p.data[:n])
	}
	return p
}

// Skip 跳过前 n 个元素，返回 p 自身。n <= 0 则不变；n 大于长度得到空切片。已出错时跳过。
func (p *Pipe[T]) Skip(n int) *Pipe[T] {
	if p.err != nil {
		return p
	}
	if n <= 0 {
		return p
	}
	if n >= len(p.data) {
		p.data = make([]T, 0)
		return p
	}
	p.data = slices.Clone(p.data[n:])
	return p
}

// SortFunc 按 cmp 原地排序（cmp 语义同 slices.SortFunc），返回 p 自身。已出错时跳过。
func (p *Pipe[T]) SortFunc(cmp func(a, b T) int) *Pipe[T] {
	if p.err == nil {
		p.data = slices.Clone(p.data)
		slices.SortFunc(p.data, cmp)
	}
	return p
}

// ToMapNe 将当前切片转为 map。相同 key 后出现的覆盖先出现的；链路已出错时返回该错误。
func (p *Pipe[T]) ToMapNe[K comparable, V any](f func(T) (K, V)) (map[K]V, error) {
	if p.err != nil {
		return nil, p.err
	}
	return ToMapNe(p.data, f), nil
}

// ToMapListNe 将当前切片按 key 分组。同一 key 的元素保持原顺序；链路已出错时返回该错误。
func (p *Pipe[T]) ToMapListNe[K comparable, V any](f func(T) (K, V)) (map[K][]V, error) {
	if p.err != nil {
		return nil, p.err
	}
	return ToMapListNe(p.data, f), nil
}

// Reduce 从 init 起依次折叠。fn 出错或链路已出错时返回零值 K 与错误。
func (p *Pipe[T]) Reduce[K any](init K, fn func(K, T) (K, error)) (K, error) {
	if p.err != nil {
		var z K
		return z, p.err
	}
	acc := init
	for _, t := range p.data {
		next, err := fn(acc, t)
		if err != nil {
			var z K
			return z, err
		}
		acc = next
	}
	return acc, nil
}

// ReduceNe 是 Reduce 的无 error 版本。链路已出错时返回零值 K 与错误。
func (p *Pipe[T]) ReduceNe[K any](init K, fn func(K, T) K) (K, error) {
	if p.err != nil {
		var z K
		return z, p.err
	}
	acc := init
	for _, t := range p.data {
		acc = fn(acc, t)
	}
	return acc, nil
}
