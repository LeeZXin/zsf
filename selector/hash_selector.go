package selector

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

type HashFunc func([]byte) uint32

var (
	// 哈希函数映射
	hashFuncMap = map[string]HashFunc{
		"crc32":   crc32.ChecksumIEEE,
		"murmur3": murmur3,
	}
)

// HashSelector 哈希路由选择器
type HashSelector struct {
	Nodes        []*Node
	HashFuncName string
	HashFunc     HashFunc
	init         bool
}

func (s *HashSelector) Init() error {
	if s.Nodes == nil || len(s.Nodes) == 0 {
		return errors.New("empty nodes")
	}
	if s.HashFunc == nil {
		if s.HashFuncName == "" {
			s.HashFuncName = "crc32"
		}
		hashFunc, ok := hashFuncMap[s.HashFuncName]
		if ok {
			s.HashFunc = hashFunc
		} else {
			return errors.New("hash func not found")
		}
	}
	s.init = true
	return nil
}

func (s *HashSelector) Select(key ...string) (*Node, error) {
	if !s.init {
		return nil, errors.New("call this after init")
	}
	sk := "noneKey"
	if key != nil && len(key) > 0 {
		sk = key[0]
	}
	h := s.HashFunc([]byte(sk))
	return s.Nodes[h%uint32(len(s.Nodes))], nil
}

func murmur3(key []byte) uint32 {
	const (
		c1 = 0xcc9e2d51
		c2 = 0x1b873593
		r1 = 15
		r2 = 13
		m  = 5
		n  = 0xe6546b64
	)
	var (
		seed = uint32(1938)
		h    = seed
		k    uint32
		l    = len(key)
		end  = l - (l % 4)
	)
	for i := 0; i < end; i += 4 {
		k = binary.LittleEndian.Uint32(key[i:])
		k *= c1
		k = (k << r1) | (k >> (32 - r1))
		k *= c2

		h ^= k
		h = (h << r2) | (h >> (32 - r2))
		h = h*m + n
	}
	k = 0
	switch l & 3 {
	case 3:
		k ^= uint32(key[end+2]) << 16
		fallthrough
	case 2:
		k ^= uint32(key[end+1]) << 8
		fallthrough
	case 1:
		k ^= uint32(key[end])
		k *= c1
		k = (k << r1) | (k >> (32 - r1))
		k *= c2
		h ^= k
	}
	h ^= uint32(l)
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16
	return h
}
