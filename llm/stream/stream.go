// Package stream 带关闭语义的单消费者流式通道（Stream[T]）：
// 生产者 Send / CloseSend，消费者 Recv / CloseRecv，双向幂等关闭。
package stream

import (
	"io"
	"sync"
)

// Stream 带关闭语义的单消费者流式通道。
//
// 生产者：Send 逐个发送数据块，结束后调用 CloseSend。
// 消费者：Recv 逐个读取，流正常结束或调用 CloseRecv 后返回 io.EOF。
// CloseSend / CloseRecv 均幂等，可安全并发调用。
// 注意：生产者调用 CloseSend 后不可再 Send（会永久阻塞）。
type Stream[T any] struct {
	items chan streamItem[T]
	done  chan struct{}

	closeSendOnce sync.Once
	closeRecvOnce sync.Once
}

type streamItem[T any] struct {
	chunk T
	err   error
}

// NewStream 创建容量为 cap 的流式通道。
func NewStream[T any](cap int) *Stream[T] {
	return &Stream[T]{
		items: make(chan streamItem[T], cap),
		done:  make(chan struct{}),
	}
}

// Recv 读取下一个数据块。
// 流正常结束（生产者 CloseSend）或消费者调用 CloseRecv 后返回 io.EOF。
func (s *Stream[T]) Recv() (T, error) {
	select {
	case item, ok := <-s.items:
		if !ok {
			var t T
			return t, io.EOF
		}
		return item.chunk, item.err
	case <-s.done:
		var t T
		return t, io.EOF
	}
}

// Send 发送一个数据块；消费者已调用 CloseRecv 时返回 true，数据被丢弃。
func (s *Stream[T]) Send(chunk T, err error) (closed bool) {
	// 先快速检查：消费者已关闭时直接返回，避免与发送 case 随机竞争。
	select {
	case <-s.done:
		return true
	default:
	}
	item := streamItem[T]{chunk: chunk, err: err}
	select {
	case <-s.done:
		return true
	case s.items <- item:
		return false
	}
}

// CloseSend 生产者结束发送，幂等。
func (s *Stream[T]) CloseSend() {
	s.closeSendOnce.Do(func() {
		close(s.items)
	})
}

// CloseRecv 消费者提前终止接收，幂等。
func (s *Stream[T]) CloseRecv() {
	s.closeRecvOnce.Do(func() {
		close(s.done)
	})
}
