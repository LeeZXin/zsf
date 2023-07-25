package bpoolutil

import (
	"bytes"
	"sync"
)

type BPool struct {
	p sync.Pool
}

func (b *BPool) Get() *bytes.Buffer {
	s := b.p.Get()
	if s == nil {
		return &bytes.Buffer{}
	}
	return s.(*bytes.Buffer)
}

func (b *BPool) Put(s *bytes.Buffer) {
	s.Reset()
	b.p.Put(s)
}
