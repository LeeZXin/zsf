// Package fileutil 提供文件系统基础工具，目前仅包含文件存在性判断。
package fileutil

import "os"

// FileExists 判断 name 指向的文件或目录是否存在：
// 存在返回 (true, nil)；不存在返回 (false, nil)；
// 其他错误（如权限不足）返回 (false, err)，错误传播给调用方。
func FileExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
