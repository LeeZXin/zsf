package apigw

// TNode 前缀树

type TrieNode[T any] struct {
	label    string
	children map[rune]*TrieNode[T]
	data     T
	has      bool
}

const (
	// LongestMatchType 最长匹配
	LongestMatchType = iota + 1
	// ShortestMatchType 最短匹配
	ShortestMatchType
)

// Trie 通用前缀匹配树
type Trie[T any] struct {
	root *TrieNode[T]
}

// Insert 插入
func (r *Trie[T]) Insert(key string, data T) {
	if r.root == nil {
		r.root = &TrieNode[T]{
			children: make(map[rune]*TrieNode[T], 8),
		}
	}
	if key == "" {
		return
	}
	node := r.root
	for i, k := range key {
		if c, ok := node.children[k]; !ok {
			c = &TrieNode[T]{
				label:    key[:i+1],
				children: make(map[rune]*TrieNode[T], 8),
			}
			node.children[k] = c
		}
		node = node.children[k]
	}
	node.data = data
	node.has = true
}

// FullSearch 完全匹配
func (r *Trie[T]) FullSearch(key string) (any, bool) {
	if r.root == nil {
		return nil, false
	}
	node := r.root
	for _, k := range key {
		var ok bool
		node, ok = node.children[k]
		if !ok {
			return nil, false
		}
	}
	if !node.has {
		return nil, false
	}
	return node.data, true
}

// PrefixSearch 前缀匹配
func (r *Trie[T]) PrefixSearch(key string, matchType int) (TrieNode[T], bool) {
	if r.root == nil {
		return TrieNode[T]{}, false
	}
	node := r.root
	list := make([]TrieNode[T], 0, 8)
	for _, k := range key {
		if node.has {
			if matchType == ShortestMatchType {
				return *node, true
			}
			list = append(list, *node)
		}
		var ok bool
		node, ok = node.children[k]
		if !ok {
			break
		}
	}
	if len(list) == 0 {
		return TrieNode[T]{}, false
	}
	//最长匹配
	return list[len(list)-1], true
}
