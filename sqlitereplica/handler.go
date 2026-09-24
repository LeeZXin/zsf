package sqlitereplica

import (
	"context"
	"errors"
	"io"
)

/*
Handler 远端存储抽象。name 为相对路径（相对 handler 自身根目录），格式：

  - "<generation>/snapshots/%08x.snapshot.lz4"  快照文件
  - "<generation>/wal/%08x_%08x.wal.lz4"        WAL 段文件

实现约定：

  - Output 返回时远端文件必须完整可见。原子性由实现保证：file handler 写临时
    文件后 rename，oss handler 走整对象上传
  - Restore 把 generation 下所有文件下载到 dir，保持与 name 相同的相对布局
    （dir/<generation>/...）。generation 为空串时恢复最新一代（字典序最大的
    合法 generation 名，即数值最大），并返回实际恢复的 generation 名
  - Remove 幂等：generation 不存在时返回 nil
*/
type Handler interface {
	// Output 将 r 内容写入远端 name，覆盖写。
	Output(ctx context.Context, name string, r io.Reader) error
	// Restore 下载 generation 下所有文件到本地 dir。generation 为空时恢复最新一代。
	Restore(ctx context.Context, generation string, dir string) (string, error)
	// Remove 删除 generation 下所有文件。
	Remove(ctx context.Context, generation string) error
}

var (
	// ErrNoGeneration 远端不存在任何 generation。
	ErrNoGeneration = errors.New("sqlitereplica: no generation available")
	// ErrNoSnapshots 远端 generation 内不存在快照。
	ErrNoSnapshots = errors.New("sqlitereplica: no snapshots available")
)

/*
IsGenerationName 判断 s 是否为合法 generation 名。

格式：16 位小写 hex（%016x 计数器格式，非 litestream 的随机 hex）。字典序
等于数值序，因此恢复最新代只需取名字最大的 generation，无需读远端时间戳。
*/
func IsGenerationName(s string) bool {
	if len(s) != 16 {
		return false
	}
	for _, ch := range s {
		if !isHexChar(ch) {
			return false
		}
	}
	return true
}

func isHexChar(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')
}
