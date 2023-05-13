package ws

import (
	"bytes"
	"sync"
)

type buffer struct {
	p sync.Pool
}

func (b *buffer) Get() *bytes.Buffer {
	s := b.p.Get()
	if s == nil {
		return &bytes.Buffer{}
	}
	return s.(*bytes.Buffer)
}

func (b *buffer) Put(s *bytes.Buffer) {
	s.Reset()
	b.p.Put(s)
}
