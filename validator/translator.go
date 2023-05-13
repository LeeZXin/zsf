package validator

import (
	"strings"
)

type Translator map[string]string

type TemplateErr struct {
	VarName  string
	Format   string
	Bindings map[string]string
	errMsg   string
}

func (t *TemplateErr) Error() string {
	return t.printString()
}

func (t *TemplateErr) printString() string {
	if t.errMsg != "" {
		return t.errMsg
	}
	format := t.Format
	bindings := t.Bindings
	builder := strings.Builder{}
	builder.WriteString(t.VarName)
	end := len(format)
	for i := 0; i < end; {
		if format[i] == '{' {
			argEnd := -1
			for j := i + 1; j < end; j++ {
				if format[j] == '{' {
					break
				}
				if format[j] == '}' {
					argEnd = j
					break
				}
			}
			if argEnd > -1 {
				arg := format[i+1 : argEnd]
				builder.WriteString(bindings[arg])
				i = argEnd + 1
			} else {
				builder.WriteByte(format[i])
				i++
			}
		} else {
			builder.WriteByte(format[i])
			i++
		}
	}
	t.errMsg = builder.String()
	return t.errMsg
}
