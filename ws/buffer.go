package ws

import (
	"bytes"
	"sync"
)

type bpool struct {
	p sync.Pool
}

func (b *bpool) Get() *bytes.Buffer {
	s := b.p.Get()
	if s == nil {
		return &bytes.Buffer{}
	}
	return s.(*bytes.Buffer)
}

func (b *bpool) Put(s *bytes.Buffer) {
	s.Reset()
	b.p.Put(s)
}
