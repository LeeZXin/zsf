// Package xormutil 提供数据库相关的工具函数：bool 与 MySQL tinyint 转换、MySQL 错误识别。
package xormutil

import (
	"errors"

	"github.com/go-sql-driver/mysql"
)

// Bool2TinyInt 将 bool 转换为 MySQL tinyint 值：true 返回 1，false 返回 0。
func Bool2TinyInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// IsMysqlDuplicatedEntryError 判断 err 是否为 MySQL 唯一键冲突（错误码 1062 Duplicate entry）。
// 使用 errors.AsType 可识别被 fmt.Errorf 等包装过的错误；err 为 nil 时返回 false。
func IsMysqlDuplicatedEntryError(err error) bool {
	if err == nil {
		return false
	}
	if merr, ok := errors.AsType[*mysql.MySQLError](err); ok {
		return merr.Number == 1062
	}
	return false
}
