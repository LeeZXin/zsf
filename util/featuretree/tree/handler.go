package tree

import (
	"errors"
	"github.com/LeeZXin/zsf/util/luautil"
	"github.com/shopspring/decimal"
	"github.com/spf13/cast"
	"regexp"
	"sync"
)

// 表达式比较器
var (
	regCache = sync.Map{}
)

type compileRegFunc func() *regexp.Regexp

// compileReg 编译正则并缓存
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

// StringFeatureHandler 字符串处理器
type StringFeatureHandler struct {
	opMap map[*Operator]Comparator[string]
}

// GetSupportedOperators 获取支持的操作符
func (m *StringFeatureHandler) GetSupportedOperators() []*Operator {
	ret := make([]*Operator, 0, len(m.opMap))
	for key := range m.opMap {
		ret = append(ret, key)
	}
	return ret
}

// GetDataType 支持的数据类型
func (m *StringFeatureHandler) GetDataType() string {
	return "string"
}

// Handle 实际处理逻辑
func (m *StringFeatureHandler) Handle(value *StringValue, operator *Operator, userValue any, ctx *FeatureAnalyseContext) (bool, error) {
	actual := cast.ToString(userValue)
	targets := operator.ValueSplitter.SplitValue(value.Value)
	return m.opMap[operator](actual, targets), nil
}

func NewStringFeatureHandler() FeatureHandler {
	return &StringFeatureHandler{
		opMap: map[*Operator]Comparator[string]{
			Eq: func(actual string, targets []string) bool {
				if targets == nil || len(targets) == 0 {
					return false
				}
				return actual == targets[0]
			},
			In: func(actual string, targets []string) bool {
				if targets == nil {
					return false
				}
				for _, target := range targets {
					if target == actual {
						return true
					}
				}
				return false
			},
			Neq: func(actual string, targets []string) bool {
				if targets == nil || len(targets) == 0 {
					return false
				}
				return actual != targets[0]
			},
			Blank: func(actual string, targets []string) bool {
				return actual == ""
			},
			NotBlank: func(actual string, targets []string) bool {
				return actual != ""
			},
			RegMatch: func(actual string, targets []string) bool {
				if targets == nil || len(targets) == 0 {
					return false
				}
				reg := compileReg(targets[0])()
				if reg == nil {
					return false
				}
				ret := reg.MatchString(actual)
				return ret
			},
		},
	}
}

// NumberFeatureHandler 数字处理器
type NumberFeatureHandler struct {
	opMap map[*Operator]Comparator[decimal.Decimal]
}

// GetSupportedOperators 获取支持的操作符
func (m *NumberFeatureHandler) GetSupportedOperators() []*Operator {
	ret := make([]*Operator, 0, len(m.opMap))
	for key := range m.opMap {
		ret = append(ret, key)
	}
	return ret
}

// GetDataType 支持的数据类型
func (m *NumberFeatureHandler) GetDataType() string {
	return "number"
}

// Handle 实际处理逻辑
func (m *NumberFeatureHandler) Handle(value *StringValue, operator *Operator, userValue any, ctx *FeatureAnalyseContext) (bool, error) {
	actual := cast.ToString(userValue)
	targets := operator.ValueSplitter.SplitValue(value.Value)
	actualDecimal, err := decimal.NewFromString(actual)
	if err != nil {
		return false, nil
	}
	targetsDecimal := make([]decimal.Decimal, 0, len(targets))
	for _, target := range targets {
		targetDecimal, err := decimal.NewFromString(target)
		if err != nil {
			return false, nil
		}
		targetsDecimal = append(targetsDecimal, targetDecimal)
	}
	return m.opMap[operator](actualDecimal, targetsDecimal), nil
}

func NewNumberFeatureHandler() FeatureHandler {
	return &NumberFeatureHandler{
		opMap: map[*Operator]Comparator[decimal.Decimal]{
			Eq: func(actual decimal.Decimal, targets []decimal.Decimal) bool {
				if targets == nil || len(targets) == 0 {
					return false
				}
				return actual.Equal(targets[0])
			},
			In: func(actual decimal.Decimal, targets []decimal.Decimal) bool {
				if targets == nil {
					return false
				}
				for _, target := range targets {
					if target.Equal(actual) {
						return true
					}
				}
				return false
			},
			Neq: func(actual decimal.Decimal, targets []decimal.Decimal) bool {
				if targets == nil || len(targets) == 0 {
					return false
				}
				return !actual.Equal(targets[0])
			},
			Gt: func(actual decimal.Decimal, targets []decimal.Decimal) bool {
				if targets == nil || len(targets) == 0 {
					return false
				}
				return actual.GreaterThan(targets[0])
			},
			Gte: func(actual decimal.Decimal, targets []decimal.Decimal) bool {
				if targets == nil || len(targets) == 0 {
					return false
				}
				return actual.GreaterThanOrEqual(targets[0])
			},
			Lt: func(actual decimal.Decimal, targets []decimal.Decimal) bool {
				if targets == nil || len(targets) == 0 {
					return false
				}
				return actual.LessThan(targets[0])
			},
			Lte: func(actual decimal.Decimal, targets []decimal.Decimal) bool {
				if targets == nil || len(targets) == 0 {
					return false
				}
				return actual.LessThanOrEqual(targets[0])
			},
			Between: func(actual decimal.Decimal, targets []decimal.Decimal) bool {
				if targets == nil || len(targets) < 2 {
					return false
				}
				return actual.GreaterThanOrEqual(targets[0]) && actual.LessThanOrEqual(targets[1])
			},
		},
	}
}

// ScriptFeatureHandler 脚本处理器
type ScriptFeatureHandler struct {
}

// GetSupportedOperators 获取支持的操作符
func (m *ScriptFeatureHandler) GetSupportedOperators() []*Operator {
	return []*Operator{
		Script,
	}
}

// GetDataType 支持的数据类型
func (m *ScriptFeatureHandler) GetDataType() string {
	return "script"
}

// Handle 实际处理逻辑
func (m *ScriptFeatureHandler) Handle(value *StringValue, operator *Operator, userValue any, ctx *FeatureAnalyseContext) (bool, error) {
	if value == nil {
		return false, errors.New("empty script config string value")
	}
	bindings := luautil.Copy2Bindings(ctx.OriginMessage)
	bindings.Set("userValue", userValue)
	proto, err := value.CachedScript.GetCompiledScript()
	if err != nil {
		return false, err
	}
	return GetDefaultScriptExecutor().ExecuteAndReturnBool(proto, bindings)
}

func NewScriptFeatureHandler() FeatureHandler {
	return &ScriptFeatureHandler{}
}

func init() {
	// 注册字符串处理器
	RegisterHandler(NewStringFeatureHandler())
	// 注册数字处理器
	RegisterHandler(NewNumberFeatureHandler())
	// 注册脚本处理器
	RegisterHandler(NewScriptFeatureHandler())
}
