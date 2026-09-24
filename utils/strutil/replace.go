package strutil

import "strings"

/*
Replacer 是链式字符串替换器：对初始字符串依次执行多次替换。
Replace 返回自身以支持链式调用，String 获取最终结果。
*/
type Replacer struct {
	str string
}

/*
NewReplacer 创建替换器，origin 为初始字符串。
*/
func NewReplacer(origin string) *Replacer {
	return &Replacer{
		str: origin,
	}
}

/*
Replace 将当前字符串中所有出现的 oldStr 替换为 newStr（等价于 strings.ReplaceAll），
返回自身以支持链式调用。
*/
func (r *Replacer) Replace(oldStr, newStr string) *Replacer {
	r.str = strings.ReplaceAll(r.str, oldStr, newStr)
	return r
}

/*
String 返回当前替换结果字符串。
*/
func (r *Replacer) String() string {
	return r.str
}
