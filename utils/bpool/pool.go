package bpool

import (
	"bytes"
	"sync"
)

type Pool struct {
	p sync.Pool
}

func (b *Pool) Get() *bytes.Buffer {
	s := b.p.Get()
	if s == nil {
		return &bytes.Buffer{}
	}
	return s.(*bytes.Buffer)
}

func (b *Pool) Put(s *bytes.Buffer) {
	s.Reset()
	b.p.Put(s)
}
