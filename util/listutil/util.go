package listutil

import (
	"errors"
	"github.com/LeeZXin/zsf/util/hashset"
	"math"
	"math/rand"
)

func Contains[T comparable](t T, arr []T) bool {
	for _, a := range arr {
		if a == t {
			return true
		}
	}
	return false
}

type Stream[T any] struct {
	data []T
}

func Filter[T any](data []T, fn func(T) (bool, error)) ([]T, error) {
	if fn == nil {
		return nil, errors.New("nil filter")
	}
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

func Map[T, K any](data []T, mapper func(T) (K, error)) ([]K, error) {
	if mapper == nil {
		return nil, errors.New("nil mapper")
	}
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

func Distinct[T comparable](data []T) []T {
	return hashset.NewHashSet(data).ToSlice()
}

func Partition[T any](data []T, size int) [][]T {
	if size <= 0 {
		return [][]T{}
	}
	if data == nil {
		return [][]T{}
	}
	ret := make([][]T, 0, int(math.Ceil(float64(len(data)/size))))
	start := 0
	for start+size < len(data) {
		ret = append(ret, data[start:start+size])
		start += size
	}
	if start < len(data) {
		ret = append(ret, data[start:])
	}
	return ret
}

func CollectToMap[T any, N comparable, K any](data []T, nameFn func(T) (N, error), valFn func(T) (K, error)) (map[N]K, error) {
	if nameFn == nil {
		return nil, errors.New("nil nameFn")
	}
	if valFn == nil {
		return nil, errors.New("nil valFn")
	}
	ret := make(map[N]K, len(data))
	for _, d := range data {
		name, err := nameFn(d)
		if err != nil {
			return nil, err
		}
		val, err := valFn(d)
		if err != nil {
			return nil, err
		}
		ret[name] = val
	}
	return ret, nil
}

func Copy[T any](data []T) []T {
	if data == nil {
		return nil
	}
	ret := make([]T, 0, len(data))
	for _, d := range data {
		ret = append(ret, d)
	}
	return ret
}

func Shuffle[T any](data []T) []T {
	if data == nil {
		return nil
	}
	data = Copy(data)
	length := len(data)
	tail := length - 1
	for i := 0; i < tail; i++ {
		j := i + rand.Intn(length-i)
		data[i], data[j] = data[j], data[i]
	}
	return data
}

func ShuffleSelf[T any](data []T) {
	if data == nil {
		return
	}
	length := len(data)
	tail := length - 1
	for i := 0; i < tail; i++ {
		j := i + rand.Intn(length-i)
		data[i], data[j] = data[j], data[i]
	}
}
