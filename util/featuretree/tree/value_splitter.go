package tree

import "strings"

var (
	// DefaultSplitter 默认分割
	DefaultSplitter = ValueSplitter{
		Delimiter: "",
	}
	// CommasSplitter 逗号分隔符
	CommasSplitter = ValueSplitter{
		Delimiter: ",",
	}
)

// ValueSplitter 字符串分割
type ValueSplitter struct {
	Delimiter string
}

func (vs *ValueSplitter) SplitValue(value string) []string {
	if vs.Delimiter == "" {
		return []string{value}
	}
	return strings.Split(value, vs.Delimiter)
}
