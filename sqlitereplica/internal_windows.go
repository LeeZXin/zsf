//go:build windows

package sqlitereplica

import (
	"os"
	"strings"
)

// fileinfo windows 不支持 uid/gid，返回 -1,-1（os.Chown 视 -1 为不变）。
func fileinfo(fi os.FileInfo) (uid, gid int) {
	return -1, -1
}

// fixRootDirectory 修正 Windows 根目录表示（"C:" → "C:\\"）。
func fixRootDirectory(p string) string {
	if len(p) == len(`\\?\c:`) {
		if os.IsPathSeparator(p[0]) && os.IsPathSeparator(p[1]) && p[2] == '?' && os.IsPathSeparator(p[3]) {
			return p + `\`
		}
	}
	return strings.ReplaceAll(p, `/`, `\`)
}
