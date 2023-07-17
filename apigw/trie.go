package apigw

// TNode 前缀树

type trieNode struct {
	label    string
	children map[rune]*trieNode
	data     any
}

const (
	// LongestMatchType 最长匹配
	LongestMatchType = iota + 1
	// ShortestMatchType 最短匹配
	ShortestMatchType
)

// Trie 通用前缀匹配树
type Trie struct {
	root *trieNode
}

// Insert 插入
func (r *Trie) Insert(key string, data any) {
	if r.root == nil {
		r.root = &trieNode{
			children: make(map[rune]*trieNode, 8),
		}
	}
	if key == "" || data == nil {
		return
	}
	node := r.root
	for i, k := range key {
		if c, ok := node.children[k]; !ok {
			c = &trieNode{
				label:    key[:i+1],
				children: make(map[rune]*trieNode, 8),
			}
			node.children[k] = c
		}
		node = node.children[k]
	}
	node.data = data
}

// FullSearch 完全匹配
func (r *Trie) FullSearch(key string) (any, bool) {
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
	if node.data == nil {
		return nil, false
	}
	return node.data, true
}

// PrefixSearch 前缀匹配
func (r *Trie) PrefixSearch(key string, matchType int) (trieNode, bool) {
	if r.root == nil {
		return trieNode{}, false
	}
	node := r.root
	list := make([]trieNode, 0, 8)
	for _, k := range key {
		if node.data != nil {
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
		return trieNode{}, false
	}
	//最长匹配
	return list[len(list)-1], true
}
