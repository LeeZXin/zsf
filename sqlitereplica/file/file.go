/*
Package file 提供 sqlitereplica.Handler 的本地目录实现，用于测试与单机备份。

远端布局：<root>/<generation>/snapshots/*.snapshot.lz4 与
<root>/<generation>/wal/*.wal.lz4。
*/
package file

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/LeeZXin/zsf/logger"

	"github.com/LeeZXin/zsf/sqlitereplica"
)

var _ sqlitereplica.Handler = (*Handler)(nil)

/*
Handler 本地目录存储：name 相对路径直接映射为 root 下的文件路径。
*/
type Handler struct {
	path string // 根目录
}

/*
NewHandler 创建本地目录 handler。目录不存在时允许（首次 Output 时按需创建），
已存在则必须是目录，否则 Fatal（fast fail，与 sqlitereplica.New 一致）。
*/
func NewHandler(path string) *Handler {
	// 已存在且不是目录：直接 Fatal，避免错误延迟到 Output/Restore 时才暴露。
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		logger.Logger.Fatal().Str("path", path).Msg("file: handler path is not a directory")
	} else if err != nil && !os.IsNotExist(err) {
		logger.Logger.Fatal().Err(err).Str("path", path).Msg("file: cannot stat handler path")
	}
	return &Handler{path: path}
}

// Path 返回根目录。
func (h *Handler) Path() string { return h.path }

/*
Output 将 r 内容写入 root/name。原子写：先写 name+".tmp"，sync 后 rename。
返回时文件完整可见。
*/
func (h *Handler) Output(_ context.Context, name string, r io.Reader) error {
	if strings.Contains(name, "..") {
		return fmt.Errorf("file: invalid name %q", name)
	}

	filename := filepath.Join(h.path, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}

	tmpFilename := filename + ".tmp"
	f, err := os.Create(tmpFilename)
	if err != nil {
		return err
	}
	// 写失败时清理临时文件，成功后由 rename 消费。
	defer func() { _ = os.Remove(tmpFilename) }()

	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpFilename, filename); err != nil {
		return err
	}
	_ = fsyncDir(filepath.Dir(filename)) // 目录落盘 best-effort
	return nil
}

/*
Restore 把 generation 下所有文件拷贝到 dir，保持相对布局
（dir/<generation>/snapshots/...、dir/<generation>/wal/...）。
generation 为空时恢复根目录下名字最大的合法 generation。
*/
func (h *Handler) Restore(ctx context.Context, generation string, dir string) (string, error) {
	if generation == "" {
		fis, err := os.ReadDir(h.path)
		if os.IsNotExist(err) {
			return "", sqlitereplica.ErrNoGeneration
		} else if err != nil {
			return "", err
		}

		var gens []string
		for _, fi := range fis {
			if fi.IsDir() && sqlitereplica.IsGenerationName(fi.Name()) {
				gens = append(gens, fi.Name())
			}
		}
		if len(gens) == 0 {
			return "", sqlitereplica.ErrNoGeneration
		}
		sort.Strings(gens)
		generation = gens[len(gens)-1] // 字典序最大 = 最新代
	}

	srcRoot := filepath.Join(h.path, generation)
	if _, err := os.Stat(srcRoot); os.IsNotExist(err) {
		return "", sqlitereplica.ErrNoGeneration
	} else if err != nil {
		return "", err
	}

	err := filepath.WalkDir(srcRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		} else if d.IsDir() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		return copyFile(path, filepath.Join(dir, generation, rel))
	})
	if err != nil {
		return "", err
	}
	return generation, nil
}

/*
Remove 删除 generation 目录，幂等。
*/
func (h *Handler) Remove(_ context.Context, generation string) error {
	if !sqlitereplica.IsGenerationName(generation) {
		return fmt.Errorf("file: invalid generation %q", generation)
	}
	if err := os.RemoveAll(filepath.Join(h.path, generation)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

/*
copyFile 拷贝文件到目标路径（自动创建父目录）。
*/
func copyFile(src, dst string) error {
	srcF, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcF.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}

	dstF, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(dstF, srcF); err != nil {
		_ = dstF.Close()
		return err
	}
	return dstF.Close()
}

/*
fsyncDir 同步目录元数据（rename 的持久化）。文件系统不支持时忽略错误。
*/
func fsyncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}
