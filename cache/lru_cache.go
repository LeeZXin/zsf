package cache

import (
	"context"
	"sync"
	"time"
)

// lru缓存 带过期时间
// 双向链表 + map

type dNode struct {
	Pre   *dNode
	Next  *dNode
	Entry *dEntry
}

func (n *dNode) addToNext(node *dNode) {
	if node.Next != nil {
		node.Next.Pre = node
	}
	node.Next = n.Next
	node.Pre = n
	n.Next = node
}

func (n *dNode) delSelf() {
	pre := n.Pre
	next := n.Next
	pre.Next = next
	if next != nil {
		next.Pre = pre
	}

	n.Pre = nil
	n.Next = nil
}

type dEntry struct {
	*SingleCacheEntry
	Node *dNode
	Key  string
}

type LRUCache struct {
	mu             sync.Mutex
	cache          map[string]*dEntry
	head           *dNode
	tail           *dNode
	maxSize        int
	supplier       SupplierWithKey
	expireDuration time.Duration
}

func NewLRUCache(supplier SupplierWithKey, duration time.Duration, maxSize int) (*LRUCache, error) {
	if maxSize <= 0 {
		return nil, IllegalMaxSizeErr
	}
	if supplier == nil {
		return nil, NilSupplierErr
	}
	if duration <= 0 {
		return nil, IllegalDurationErr
	}
	defaultNode := &dNode{
		Pre:   nil,
		Next:  nil,
		Entry: nil,
	}
	return &LRUCache{
		mu:             sync.Mutex{},
		cache:          make(map[string]*dEntry, 8),
		head:           defaultNode,
		tail:           defaultNode,
		maxSize:        maxSize,
		supplier:       supplier,
		expireDuration: duration,
	}, nil
}

func (c *LRUCache) LoadData(ctx context.Context, key string) (any, error) {
	getEntry := func() (*dEntry, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		entry, ok := c.getKey(key)
		if ok {
			entry.Node.delSelf()
			c.addToTail(entry)
			return entry, nil
		}
		singleEntry, err := NewSingleCacheEntry(func(ctx context.Context) (any, error) {
			return c.supplier(ctx, key)
		}, c.expireDuration)
		if err != nil {
			return nil, err
		}
		node := &dNode{}
		entry = &dEntry{
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
	entry, err := getEntry()
	if err != nil {
		return nil, err
	}
	return entry.LoadData(ctx)
}

// addToTail 添加到尾部
func (c *LRUCache) addToTail(entry *dEntry) {
	c.tail.addToNext(entry.Node)
	c.tail = entry.Node
}

// removeEldestKey 移除最老的key
func (c *LRUCache) removeEldestKey() {
	eldest := c.head.Next
	if eldest == nil {
		return
	}
	key := eldest.Entry.Key
	delete(c.cache, key)
	eldest.delSelf()
}

func (c *LRUCache) AllKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.cache))
	for key := range c.cache {
		k := key
		keys = append(keys, k)
	}
	return keys
}

func (c *LRUCache) getKey(key string) (*dEntry, bool) {
	ret, ok := c.cache[key]
	return ret, ok
}

func (c *LRUCache) RemoveKey(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.getKey(key)
	if ok {
		delete(c.cache, key)
		entry.Node.delSelf()
	}
}

func (c *LRUCache) ContainsKey(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.getKey(key)
	return ok
}

func (c *LRUCache) Clear() {
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
