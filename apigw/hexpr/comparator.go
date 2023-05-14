package hexpr

import (
	"regexp"
	"strings"
	"sync"
)

// 表达式比较器
var (
	regCache = sync.Map{}
)

var (
	// EqCpr 等于
	EqCpr = &EqComparator{}
	// NeqCpr 不等于
	NeqCpr = &NeqComparator{}
	// InCpr 属于
	InCpr = &InComparator{}
	// ContainsCpr 存在
	ContainsCpr = &ContainsComparator{}
	// RegCpr 正则
	RegCpr = &RegComparator{}
	// EmptyCpr 为空
	EmptyCpr = &EmptyComparator{}
	// NotEmptyCpr 不为空
	NotEmptyCpr = &NotEmptyComparator{}
)

type Comparator interface {
	Compare(data string, target []string) bool
}

type EqComparator struct{}

func (c *EqComparator) Compare(data string, target []string) bool {
	if target == nil || len(target) == 0 {
		return false
	}
	return data == target[0]
}

type NeqComparator struct{}

func (c *NeqComparator) Compare(data string, target []string) bool {
	if target == nil || len(target) == 0 {
		return data != ""
	}
	return data != target[0]
}

type InComparator struct{}

func (c *InComparator) Compare(data string, target []string) bool {
	if target == nil || len(target) == 0 {
		return false
	}
	for _, s := range target {
		if s == data {
			return true
		}
	}
	return false
}

type ContainsComparator struct{}

func (c *ContainsComparator) Compare(data string, target []string) bool {
	if target == nil || len(target) == 0 {
		return false
	}
	return strings.Contains(target[0], data)
}

type EmptyComparator struct{}

func (c *EmptyComparator) Compare(data string, target []string) bool {
	return data == ""
}

type NotEmptyComparator struct{}

func (c *NotEmptyComparator) Compare(data string, target []string) bool {
	return data != ""
}

type RegComparator struct{}

type compileRegFunc func() *regexp.Regexp

func (c *RegComparator) Compare(data string, target []string) bool {
	if data == "" || target == nil || len(target) == 0 {
		return false
	}
	reg := compileReg(target[0])()
	if reg == nil {
		return false
	}
	return reg.MatchString(data)
}

func compileReg(expr string) compileRegFunc {
	var (
		wg sync.WaitGroup
		f  compileRegFunc
	)
	wg.Add(1)
	i, loaded := regCache.LoadOrStore(expr, compileRegFunc(func() *regexp.Regexp {
		wg.Wait()
		return f()
	}))
	if loaded {
		return i.(compileRegFunc)
	}
	compile, err := regexp.Compile(expr)
	if err == nil {
		f = func() *regexp.Regexp {
			return compile
		}
		regCache.Store(expr, f)
	} else {
		f = func() *regexp.Regexp {
			return nil
		}
		regCache.Delete(expr)
	}
	wg.Done()
	return f
}
