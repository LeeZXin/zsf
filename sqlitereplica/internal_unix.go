//go:build unix

package sqlitereplica

import (
	"os"
	"syscall"
)

/*
fileinfo 返回 fi 的 uid/gid，fi 为 nil 时返回 -1,-1（os.Chown 视 -1 为不变）。
仅 unix 平台支持，windows 见 internal_windows.go。
*/
func fileinfo(fi os.FileInfo) (uid, gid int) {
	if fi == nil {
		return -1, -1
	}
	stat := fi.Sys().(*syscall.Stat_t)
	return int(stat.Uid), int(stat.Gid)
}

// fixRootDirectory unix 下无需修正根目录。
func fixRootDirectory(p string) string {
	return p
}
