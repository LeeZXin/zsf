// 本文件定义逗号分隔列表类型 List 及其转换方法，
// 用于数据库中以单字段存储多个值的场景，如标签、ID 集合等。
package model

import "strings"

// Comas 列表项分隔符（逗号）
const Comas = ","

// List 逗号分隔的列表字符串类型。
// 提供 Split/ToList 方法便于在字符串切片和逗号分隔字符串之间互相转换。
type List string

// Split 将逗号分隔的列表拆分为字符串切片。
// 如果列表为空字符串，返回 nil。
// 返回值: 拆分后的字符串切片
func (l List) Split() []string {
	if len(l) == 0 {
		return nil
	}
	return strings.Split(string(l), Comas)
}

// ToList 将字符串切片转换为逗号分隔的 List。
// 如果切片为空，返回空字符串。
// 参数:
//   - arr: 字符串切片
//
// 返回值: 逗号分隔的 List 字符串
func ToList(arr []string) List {
	if len(arr) == 0 {
		return ""
	}
	return List(strings.Join(arr, Comas))
}
