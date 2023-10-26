package hexpr

import (
	"errors"
	"github.com/gin-gonic/gin"
	"strings"
)

// 表达式
var (
	EqOperator = &Operator{
		Name: "等于",
		Op:   "eq",
		Vp:   noneSplitter,
	}
	NeqOperator = &Operator{
		Name: "不等于",
		Op:   "neq",
		Vp:   noneSplitter,
	}
	InOperator = &Operator{
		Name: "包含",
		Op:   "in",
		Vp:   commasSplitter,
	}
	ContainsOperator = &Operator{
		Name: "是否字串",
		Op:   "contains",
		Vp:   noneSplitter,
	}
	RegOperator = &Operator{
		Name: "正则匹配",
		Op:   "reg",
		Vp:   noneSplitter,
	}
	EmptyOperator = &Operator{
		Name: "空值",
		Op:   "empty",
		Vp:   noneSplitter,
	}
	NotEmptyOperator = &Operator{
		Name: "非空值",
		Op:   "notEmpty",
		Vp:   noneSplitter,
	}
)

var (
	commasSplitter = &ValueSplitter{
		Splitter: ",",
	}
	noneSplitter = &NoneSplitter{}
)

type Splitter interface {
	SplitValue(value string) []string
}

type ValueSplitter struct {
	Name     string
	Splitter string
}

func (vs ValueSplitter) SplitValue(value string) []string {
	return strings.Split(value, vs.Splitter)
}

type NoneSplitter struct{}

func (vs NoneSplitter) SplitValue(value string) []string {
	return []string{value}
}

type Operator struct {
	Name string
	Op   string
	Vp   Splitter
}

type ELeaf struct {
	Source      string
	Key         string
	Operator    *Operator
	ExpectValue string
}

type ENode struct {
	And  []ENode
	Or   []ENode
	Leaf *ELeaf
}

func (e *ENode) IsLeaf() bool {
	return e.Leaf != nil
}

type Expr struct {
	Tree ENode
}

func (e *Expr) Execute(c *gin.Context) bool {
	return executeNode(e.Tree, c)
}

func executeNode(node ENode, c *gin.Context) bool {
	if node.IsLeaf() {
		return executeLeaf(node.Leaf, c)
	} else {
		and := node.And
		or := node.Or
		if and != nil && len(and) > 0 {
			for _, n := range and {
				if !executeNode(n, c) {
					return false
				}
			}
			return true
		} else if or != nil && len(or) > 0 {
			for _, n := range or {
				if executeNode(n, c) {
					return true
				}
			}
			return false
		}
	}
	return false
}

func executeLeaf(leaf *ELeaf, c *gin.Context) bool {
	fetcher, _ := fetcherMap.Load(leaf.Source)
	val := fetcher.(Fetcher)(c, leaf.Key)
	return handler.Handle(leaf.ExpectValue, leaf.Operator, val)
}

type PlainInfo struct {
	Source   string      `json:"source"`
	Key      string      `json:"key"`
	Operator string      `json:"operator"`
	Value    string      `json:"value"`
	And      []PlainInfo `json:"and"`
	Or       []PlainInfo `json:"or"`
}

func (p *PlainInfo) Validate() error {
	_, ok := fetcherMap.Load(p.Source)
	if !ok {
		return errors.New("wrong source")
	}
	if p.Key == "" {
		return errors.New("empty key")
	}
	if p.Operator == "" {
		return errors.New("empty operator")
	}
	operators := handler.GetSupportedOperators()
	findOp := false
	for _, operator := range operators {
		if operator.Op == p.Operator {
			findOp = true
			break
		}
	}
	if !findOp {
		return errors.New("unsupported operator")
	}
	return nil
}

func (p *PlainInfo) IsLeaf() bool {
	return (p.And == nil || len(p.And) == 0) && (p.Or == nil || len(p.Or) == 0)
}

func buildLeaf(info PlainInfo) (node ENode, err error) {
	var operator *Operator = nil
	for _, o := range handler.GetSupportedOperators() {
		if o.Op == info.Operator {
			operator = o
			break
		}
	}
	if operator == nil {
		err = errors.New("unsupported operator")
		return
	}
	_, ok := fetcherMap.Load(info.Source)
	if !ok {
		err = errors.New("wrong source")
		return
	}
	node = ENode{
		Leaf: &ELeaf{
			Source:      info.Source,
			Key:         info.Key,
			Operator:    operator,
			ExpectValue: info.Value,
		},
	}
	return
}

func BuildExpr(info PlainInfo) (expr Expr, err error) {
	node, err := buildENode(info)
	if err != nil {
		return
	}
	expr = Expr{
		Tree: node,
	}
	return
}

func buildENode(info PlainInfo) (ENode, error) {
	if info.IsLeaf() {
		return buildLeaf(info)
	} else {
		var and []ENode
		var or []ENode
		if len(info.And) > 0 {
			and = make([]ENode, 0, 8)
			for _, plainInfo := range info.And {
				node, err := buildENode(plainInfo)
				if err != nil {
					return ENode{}, err
				}
				and = append(and, node)
			}
		} else if len(info.Or) > 0 {
			or = make([]ENode, 0, 8)
			for _, plainInfo := range info.Or {
				node, err := buildENode(plainInfo)
				if err != nil {
					return ENode{}, err
				}
				or = append(or, node)
			}
		} else {
			return ENode{}, errors.New("empty nodes")
		}
		return ENode{
			And:  and,
			Or:   or,
			Leaf: nil,
		}, nil
	}
}
