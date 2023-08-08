package cache

import (
	"context"
	"sync"
	"time"
)

// lru缓存 带过期时间
// 双向链表 + map

type dNode[T any] struct {
	Pre   *dNode[T]
	Next  *dNode[T]
	Entry *dEntry[T]
}

func (n *dNode[T]) addToNext(node *dNode[T]) {
	if node.Next != nil {
		node.Next.Pre = node
	}
	node.Next = n.Next
	node.Pre = n
	n.Next = node
}

func (n *dNode[T]) delSelf() {
	pre := n.Pre
	next := n.Next
	pre.Next = next
	if next != nil {
		next.Pre = pre
	}

	n.Pre = nil
	n.Next = nil
}

type dEntry[T any] struct {
	*SingleCacheEntry[T]
	Node *dNode[T]
	Key  string
}

type LRUCache[T any] struct {
	mu             sync.Mutex
	cache          map[string]*dEntry[T]
	head           *dNode[T]
	tail           *dNode[T]
	maxSize        int
	supplier       SupplierWithKey[T]
	expireDuration time.Duration
}

func NewLRUCache[T any](supplier SupplierWithKey[T], duration time.Duration, maxSize int) (ExpireCache[T], error) {
	if maxSize <= 0 {
		return nil, IllegalMaxSizeErr
	}
	if supplier == nil {
		return nil, NilSupplierErr
	}
	if duration <= 0 {
		return nil, IllegalDurationErr
	}
	defaultNode := &dNode[T]{
		Pre:   nil,
		Next:  nil,
		Entry: nil,
	}
	return &LRUCache[T]{
		mu:             sync.Mutex{},
		cache:          make(map[string]*dEntry[T], 8),
		head:           defaultNode,
		tail:           defaultNode,
		maxSize:        maxSize,
		supplier:       supplier,
		expireDuration: duration,
	}, nil
}

func (c *LRUCache[T]) LoadData(ctx context.Context, key string) (T, error) {
	getEntry := func() (*dEntry[T], error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		entry, ok := c.getKey(key)
		if ok {
			entry.Node.delSelf()
			c.addToTail(entry)
			return entry, nil
		}
		singleEntry, err := NewSingleCacheEntry(func(ctx context.Context) (T, error) {
			return c.supplier(ctx, key)
		}, c.expireDuration)
		if err != nil {
			return nil, err
		}
		node := &dNode[T]{}
		entry = &dEntry[T]{
			Key:              key,
			Node:             node,
			SingleCacheEntry: singleEntry,
		}
		node.Entry = entry
		if len(c.cache)+1 > c.maxSize {
			c.removeEldestKey()
		}
		c.addToTail(entry)
		c.cache[key] = entry
		return entry, nil
	}
	var ret T
	entry, err := getEntry()
	if err != nil {
		return ret, err
	}
	return entry.LoadData(ctx)
}

// addToTail 添加到尾部
func (c *LRUCache[T]) addToTail(entry *dEntry[T]) {
	c.tail.addToNext(entry.Node)
	c.tail = entry.Node
}

// removeEldestKey 移除最老的key
func (c *LRUCache[T]) removeEldestKey() {
	eldest := c.head.Next
	if eldest == nil {
		return
	}
	key := eldest.Entry.Key
	delete(c.cache, key)
	eldest.delSelf()
}

func (c *LRUCache[T]) AllKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.cache))
	for key := range c.cache {
		k := key
		keys = append(keys, k)
	}
	return keys
}

func (c *LRUCache[T]) getKey(key string) (*dEntry[T], bool) {
	ret, ok := c.cache[key]
	return ret, ok
}

func (c *LRUCache[T]) RemoveKey(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.getKey(key)
	if ok {
		delete(c.cache, key)
		entry.Node.delSelf()
	}
}

func (c *LRUCache[T]) ContainsKey(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.getKey(key)
	return ok
}

func (c *LRUCache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.cache {
		delete(c.cache, key)
	}
	node := c.head.Next
	for node != nil {
		tmp := node.Next
		node.delSelf()
		node = tmp
	}
}
