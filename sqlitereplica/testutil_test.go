package sqlitereplica

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

/*
replicaPos 返回 replica 当前已上传位置（测试用；Pos 方法已随公开 API 收敛
删除，测试通过包内访问 + mu 保护直接读字段）。
*/
func replicaPos(r *replica) walPos {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pos
}

/*
testHandler 内部测试用 Handler：行为与 file 子包一致（本地目录存储）。

内部测试包不能 import file 子包（file 反向依赖本包，测试中构成导入环），
故用此极简实现作为测试双。
*/
type testHandler struct {
	dir string
}

var _ Handler = (*testHandler)(nil)

func newTestHandler(dir string) *testHandler {
	return &testHandler{dir: dir}
}

func (h *testHandler) Output(_ context.Context, name string, r io.Reader) error {
	if strings.Contains(name, "..") {
		return fmt.Errorf("testHandler: invalid name %q", name)
	}
	filename := filepath.Join(h.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (h *testHandler) Restore(_ context.Context, generation string, dir string) (string, error) {
	if generation == "" {
		fis, err := os.ReadDir(h.dir)
		if os.IsNotExist(err) {
			return "", ErrNoGeneration
		} else if err != nil {
			return "", err
		}
		var gens []string
		for _, fi := range fis {
			if fi.IsDir() && IsGenerationName(fi.Name()) {
				gens = append(gens, fi.Name())
			}
		}
		if len(gens) == 0 {
			return "", ErrNoGeneration
		}
		sort.Strings(gens)
		generation = gens[len(gens)-1]
	}

	srcRoot := filepath.Join(h.dir, generation)
	if _, err := os.Stat(srcRoot); os.IsNotExist(err) {
		return "", ErrNoGeneration
	} else if err != nil {
		return "", err
	}

	err := filepath.WalkDir(srcRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		} else if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, generation, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			return err
		}
		return copyTestFile(path, dst)
	})
	if err != nil {
		return "", err
	}
	return generation, nil
}

func (h *testHandler) Remove(_ context.Context, generation string) error {
	if !IsGenerationName(generation) {
		return fmt.Errorf("testHandler: invalid generation %q", generation)
	}
	if err := os.RemoveAll(filepath.Join(h.dir, generation)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func copyTestFile(src, dst string) error {
	srcF, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcF.Close() }()

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
