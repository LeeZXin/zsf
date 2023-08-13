package tree

var (
	Eq = &Operator{
		Operator:      "eq",
		Alias:         "等于",
		ValueSplitter: DefaultSplitter,
	}
	Neq = &Operator{
		Operator:      "neq",
		Alias:         "不等于",
		ValueSplitter: DefaultSplitter,
	}
	Gt = &Operator{
		Operator:      "gt",
		Alias:         "大于",
		ValueSplitter: DefaultSplitter,
	}
	Gte = &Operator{
		Operator:      "gte",
		Alias:         "大于等于",
		ValueSplitter: DefaultSplitter,
	}
	Lt = &Operator{
		Operator:      "lt",
		Alias:         "小于",
		ValueSplitter: DefaultSplitter,
	}
	Lte = &Operator{
		Operator:      "lte",
		Alias:         "小于等于",
		ValueSplitter: DefaultSplitter,
	}
	In = &Operator{
		Operator:      "in",
		Alias:         "包含",
		ValueSplitter: CommasSplitter,
	}
	Blank = &Operator{
		Operator:      "blank",
		Alias:         "为空",
		ValueSplitter: DefaultSplitter,
	}
	NotBlank = &Operator{
		Operator:      "notBlank",
		Alias:         "不为空",
		ValueSplitter: DefaultSplitter,
	}
	RegMatch = &Operator{
		Operator:      "regMatch",
		Alias:         "正则匹配",
		ValueSplitter: DefaultSplitter,
	}
	Between = &Operator{
		Operator:      "between",
		Alias:         "范围",
		ValueSplitter: CommasSplitter,
	}
	Script = &Operator{
		Operator:      "script",
		Alias:         "脚本",
		ValueSplitter: DefaultSplitter,
	}
)

// Operator 运算操作符
type Operator struct {
	Operator      string
	Alias         string
	ValueSplitter ValueSplitter
}
