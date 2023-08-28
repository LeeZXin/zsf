package selector

import (
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
)

type HashFunc func([]byte) uint32

// HashSelector 哈希路由选择器
type HashSelector[T any] struct {
	Nodes    []Node[T]
	HashFunc HashFunc
}

func (s *HashSelector[T]) init() error {
	if s.Nodes == nil || len(s.Nodes) == 0 {
		return errors.New("empty nodes")
	}
	if s.HashFunc == nil {
		s.HashFunc = crc32.ChecksumIEEE
	}
	return nil
}

func (s *HashSelector[T]) Select(ctx context.Context, key ...string) (Node[T], error) {
	sk := "noneKey"
	if key != nil && len(key) > 0 {
		sk = key[0]
	}
	h := s.HashFunc([]byte(sk))
	return s.Nodes[h%uint32(len(s.Nodes))], nil
}

func NewHashSelector[T any](nodes []Node[T]) (Selector[T], error) {
	if nodes == nil || len(nodes) == 0 {
		return nil, EmptyNodesErr
	} else if len(nodes) == 1 {
		return &SingleNodeSelector[T]{Node: nodes[0]}, nil
	}
	h := &HashSelector[T]{Nodes: nodes}
	err := h.init()
	if err != nil {
		return nil, err
	}
	return h, nil
}

func Murmur3(key []byte) uint32 {
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
