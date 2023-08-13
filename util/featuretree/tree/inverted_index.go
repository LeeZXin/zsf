package tree

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// InvertedIndex 倒排索引
type InvertedIndex struct {
	// 特征树
	FeatureTree *FeatureTree `json:"featureTree"`
	// 特征数量
	IndexCount int `json:"indexCount"`
	// 标识
	Key string `json:"key"`
	// 倒排索引
	Indexes []string `json:"indexes"`
}

type IndexFeatureTree struct {
	FeatureTree *FeatureTree
	IndexCount  int
	Key         string
	Target      string
}

// BuildInvertedIndexes 构建倒排索引
func BuildInvertedIndexes(trees []*FeatureTree) ([]*InvertedIndex, error) {
	if trees == nil {
		return []*InvertedIndex{}, nil
	}
	ft := make([]*IndexFeatureTree, 0, len(trees))
	for _, featureTree := range trees {
		dnf, err := parseDNFExpression(featureTree.Node)
		if err != nil {
			return nil, err
		}
		for _, leaves := range dnf {
			ft = append(ft, buildIndexFeatureTree(leaves, featureTree.Id))
		}
	}
	return doCreateIndex(ft), nil
}

func doCreateIndex(trees []*IndexFeatureTree) []*InvertedIndex {
	ret := make([]*InvertedIndex, 0, len(trees))
	fm := make(map[string]*IndexFeatureTree, len(trees))
	tm := make(map[string][]string, len(trees))
	for _, iTree := range trees {
		fm[iTree.Key] = iTree
		tm[iTree.Key] = append(tm[iTree.Key], iTree.Target)
	}
	for _, t := range fm {
		ret = append(ret, &InvertedIndex{
			FeatureTree: t.FeatureTree,
			IndexCount:  t.IndexCount,
			Key:         t.Key,
			Indexes:     tm[t.Key],
		})
	}
	return ret
}

func buildIndexFeatureTree(leaves []*Leaf, target string) *IndexFeatureTree {
	keyArr := make([]string, 0, len(leaves))
	for _, leaf := range leaves {
		m, _ := json.Marshal(leaf)
		keyArr = append(keyArr, string(m))
	}
	sort.SliceStable(keyArr, func(i, j int) bool {
		return strings.Compare(keyArr[i], keyArr[j]) > 0
	})
	return &IndexFeatureTree{
		FeatureTree: buildAndFeatureTree(leaves),
		IndexCount:  len(leaves),
		Key:         strings.Join(keyArr, "#"),
		Target:      target,
	}
}

func buildAndFeatureTree(leaves []*Leaf) *FeatureTree {
	and := make([]*Node, 0, len(leaves))
	for _, leaf := range leaves {
		and = append(and, &Node{
			Leaf: leaf,
		})
	}
	return &FeatureTree{
		Node: &Node{
			And: and,
		},
	}
}

// parseDNFExpression 析取范式转化
func parseDNFExpression(node *Node) ([][]*Leaf, error) {
	if node == nil {
		return nil, errors.New("nil node")
	}
	if node.IsLeave() {
		return [][]*Leaf{
			{node.Leaf},
		}, nil
	}
	var (
		lls [][]*Leaf
	)
	if !checkEmpty(node.And) {
		for _, n := range node.And {
			dnf, err := parseDNFExpression(n)
			if err != nil {
				return nil, err
			}
			if lls == nil {
				lls = dnf
			} else {
				lls, err = crossJoin(lls, dnf)
				if err != nil {
					return nil, err
				}
			}
		}
	} else if !checkEmpty(node.Or) {
		lls = make([][]*Leaf, 0)
		for _, n := range node.Or {
			dnf, err := parseDNFExpression(n)
			if err != nil {
				return nil, err
			}
			lls = append(lls, dnf...)
		}
	}
	return lls, nil
}

// crossJoin 笛卡尔积
func crossJoin(v [][]*Leaf, v1 [][]*Leaf) ([][]*Leaf, error) {
	if checkEmpty(v) || checkEmpty(v1) {
		return nil, errors.New("empty leaf list")
	}
	ret := make([][]*Leaf, 0, len(v)*len(v1))
	for i := range v {
		for i1 := range v1 {
			t := make([]*Leaf, 0, len(v[i])+len(v1[i1]))
			t = append(t, v[i]...)
			t = append(t, v1[i1]...)
			ret = append(ret, t)
		}
	}
	return ret, nil
}

func checkEmpty[T any](v []T) bool {
	return v == nil || len(v) == 0
}
