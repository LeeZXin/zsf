package sqlitereplica

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

/*
createFile 创建文件并匹配 fi 的 mode/uid/gid（fi 为 nil 时用 0600）。
*/
func createFile(filename string, fi os.FileInfo) (*os.File, error) {
	mode := os.FileMode(0600)
	if fi != nil {
		mode = fi.Mode()
	}

	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return nil, err
	}

	uid, gid := fileinfo(fi)
	_ = f.Chown(uid, gid)
	return f, nil
}

/*
mkdirAll 与 os.mkdirAll 相同，但每个新建目录的 mode/uid/gid 匹配 fi。
meta 目录与 shadow 文件需要跟随数据库文件自身的权限，避免多用户环境下的
访问问题。
*/
func mkdirAll(path string, fi os.FileInfo) error {
	uid, gid := fileinfo(fi)

	// 快速路径：目录已存在。
	dir, err := os.Stat(path)
	if err == nil {
		if dir.IsDir() {
			return nil
		}
		return &os.PathError{Op: "mkdir", Path: path, Err: syscall.ENOTDIR}
	}

	// 慢速路径：逐级创建父目录。
	i := len(path)
	for i > 0 && os.IsPathSeparator(path[i-1]) { // 跳过尾部路径分隔符。
		i--
	}

	j := i
	for j > 0 && !os.IsPathSeparator(path[j-1]) { // 回扫出一个路径段。
		j--
	}

	if j > 1 {
		// 先创建父目录。
		if err = mkdirAll(fixRootDirectory(path[:j-1]), fi); err != nil {
			return err
		}
	}

	// 父目录已存在，创建本目录。
	mode := os.FileMode(0700)
	if fi != nil {
		mode = fi.Mode()
	}
	err = os.Mkdir(path, mode)
	if err != nil {
		// 处理 "foo/." 这类参数：目录已存在则视为成功。
		dir, err1 := os.Lstat(path)
		if err1 == nil && dir.IsDir() {
			_ = os.Chown(path, uid, gid)
			return nil
		}
		return err
	}
	_ = os.Chown(path, uid, gid)
	return nil
}

/*
removeTmpFiles 递归删除 root 下的 *.tmp 残留文件（上次崩溃遗留）。
*/
func removeTmpFiles(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过读不出来的文件
		} else if info.IsDir() {
			return nil // 跳过目录
		} else if !strings.HasSuffix(path, ".tmp") {
			return nil // 跳过非临时文件
		}
		return os.Remove(path)
	})
}

/*
rollback 回滚事务，忽略「已提交或已回滚」错误。
*/
func rollback(tx *sql.Tx) error {
	if err := tx.Rollback(); err != nil && !strings.Contains(err.Error(), `transaction has already been committed or rolled back`) {
		return err
	}
	return nil
}
