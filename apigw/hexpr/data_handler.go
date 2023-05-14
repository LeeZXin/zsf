package hexpr

// 数据处理器
var (
	handler DataHandler
)

func init() {
	handler = &StringHandler{}
	handler.Init()
}

type DataHandler interface {
	// Init 初始化
	Init()
	// GetSupportedOperators 获取支持的操作符
	GetSupportedOperators() []*Operator
	// Handle 实际处理逻辑
	Handle(expectValue string, operator *Operator, actualValue string) bool
}

type StringHandler struct {
	action map[*Operator]Comparator
}

func (h *StringHandler) Init() {
	h.action = map[*Operator]Comparator{
		EqOperator:       EqCpr,
		NeqOperator:      NeqCpr,
		InOperator:       InCpr,
		ContainsOperator: ContainsCpr,
		RegOperator:      RegCpr,
		EmptyOperator:    EmptyCpr,
		NotEmptyOperator: NotEmptyCpr,
	}
}

func (h *StringHandler) GetSupportedOperators() []*Operator {
	return []*Operator{
		EqOperator,
		NeqOperator,
		InOperator,
		ContainsOperator,
		RegOperator,
		EmptyOperator,
		NotEmptyOperator,
	}
}

func (h *StringHandler) Handle(expectValue string, operator *Operator, actualValue string) bool {
	if h.action == nil {
		return false
	}
	comparator, ok := h.action[operator]
	if !ok {
		return false
	}
	return comparator.Compare(actualValue, operator.Vp.SplitValue(expectValue))
}
