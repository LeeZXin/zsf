package selector

import (
	"errors"
	"hash/crc32"
	"hash/fnv"
	"sort"
	"strconv"
	"sync"
)

type uints []uint32

func (x uints) Len() int { return len(x) }

func (x uints) Less(i, j int) bool { return x[i] < x[j] }

func (x uints) Swap(i, j int) { x[i], x[j] = x[j], x[i] }

var ErrEmptyCircle = errors.New("empty circle")

type ConsistentHashSelector[T any] struct {
	circle           map[uint32]string
	members          map[string]bool
	data             map[string]T
	sortedHashes     uints
	NumberOfReplicas int
	count            int64
	scratch          [64]byte
	UseFnv           bool
	sync.RWMutex
}

func NewConsistentHashSelector[T any](numberOfReplicas int) *ConsistentHashSelector[T] {
	c := new(ConsistentHashSelector[T])
	if numberOfReplicas > 0 {
		c.NumberOfReplicas = numberOfReplicas
	} else {
		c.NumberOfReplicas = 10
	}
	c.circle = make(map[uint32]string)
	c.members = make(map[string]bool)
	c.data = make(map[string]T)
	return c
}

func (c *ConsistentHashSelector[T]) eltKey(elt string, idx int) string {
	return strconv.Itoa(idx) + elt
}

func (c *ConsistentHashSelector[T]) Add(elt string, data T) {
	c.Lock()
	defer c.Unlock()
	c.add(elt, data)
}

func (c *ConsistentHashSelector[T]) add(elt string, data T) {
	for i := 0; i < c.NumberOfReplicas; i++ {
		c.circle[c.hashKey(c.eltKey(elt, i))] = elt
	}
	c.members[elt] = true
	c.data[elt] = data
	c.updateSortedHashes()
	c.count++
}

func (c *ConsistentHashSelector[T]) Remove(elt string) {
	c.Lock()
	defer c.Unlock()
	c.remove(elt)
}

func (c *ConsistentHashSelector[T]) remove(elt string) {
	for i := 0; i < c.NumberOfReplicas; i++ {
		delete(c.circle, c.hashKey(c.eltKey(elt, i)))
	}
	delete(c.members, elt)
	delete(c.data, elt)
	c.updateSortedHashes()
	c.count--
}

func (c *ConsistentHashSelector[T]) Set(data map[string]T) {
	c.Lock()
	defer c.Unlock()
	for k := range c.members {
		found := false
		for v := range data {
			if k == v {
				found = true
				break
			}
		}
		if !found {
			c.remove(k)
		}
	}
	for v := range data {
		_, exists := c.members[v]
		if exists {
			continue
		}
		c.add(v, data[v])
	}
}

func (c *ConsistentHashSelector[T]) Members() []string {
	c.RLock()
	defer c.RUnlock()
	var m []string
	for k := range c.members {
		m = append(m, k)
	}
	return m
}

func (c *ConsistentHashSelector[T]) Get(name string) (T, error) {
	c.RLock()
	defer c.RUnlock()
	if len(c.circle) == 0 {
		var t T
		return t, ErrEmptyCircle
	}
	key := c.hashKey(name)
	i := c.search(key)
	return c.data[c.circle[c.sortedHashes[i]]], nil
}

func (c *ConsistentHashSelector[T]) GetN(name string, n int) ([]T, error) {
	c.RLock()
	defer c.RUnlock()

	if len(c.circle) == 0 {
		return nil, ErrEmptyCircle
	}

	if c.count < int64(n) {
		n = int(c.count)
	}

	var (
		key   = c.hashKey(name)
		i     = c.search(key)
		start = i
		res   = make([]string, 0, n)
		elem  = c.circle[c.sortedHashes[i]]
	)

	res = append(res, elem)

	if len(res) != n {
		for i = start + 1; i != start; i++ {
			if i >= len(c.sortedHashes) {
				i = 0
			}
			elem = c.circle[c.sortedHashes[i]]
			if !sliceContainsMember(res, elem) {
				res = append(res, elem)
			}
			if len(res) == n {
				break
			}
		}
	}

	ret := make([]T, 0, n)

	for _, r := range res {
		ret = append(ret, c.data[r])
	}

	return ret, nil
}

func (c *ConsistentHashSelector[T]) search(key uint32) (i int) {
	f := func(x int) bool {
		return c.sortedHashes[x] > key
	}
	i = sort.Search(len(c.sortedHashes), f)
	if i >= len(c.sortedHashes) {
		i = 0
	}
	return
}

func (c *ConsistentHashSelector[T]) hashKey(key string) uint32 {
	if c.UseFnv {
		return c.hashKeyFnv(key)
	}
	return c.hashKeyCRC32(key)
}

func (c *ConsistentHashSelector[T]) hashKeyCRC32(key string) uint32 {
	if len(key) < 64 {
		var scratch [64]byte
		copy(scratch[:], key)
		return crc32.ChecksumIEEE(scratch[:len(key)])
	}
	return crc32.ChecksumIEEE([]byte(key))
}

func (c *ConsistentHashSelector[T]) hashKeyFnv(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32()
}

func (c *ConsistentHashSelector[T]) updateSortedHashes() {
	hashes := c.sortedHashes[:0]
	if cap(c.sortedHashes)/(c.NumberOfReplicas*4) > len(c.circle) {
		hashes = nil
	}
	for k := range c.circle {
		hashes = append(hashes, k)
	}
	sort.Sort(hashes)
	c.sortedHashes = hashes
}

func sliceContainsMember(set []string, member string) bool {
	for _, m := range set {
		if m == member {
			return true
		}
	}
	return false
}
