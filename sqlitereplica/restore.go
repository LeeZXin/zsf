package sqlitereplica

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/LeeZXin/zsf/logger"

	"github.com/pierrec/lz4/v4"
)

/*
RestoreOptions 恢复参数。
*/
type RestoreOptions struct {
	// OutputPath 恢复目标路径，必填且必须不存在。
	OutputPath string
	// Generation 要恢复的 generation 名；为空时恢复最新一代。
	Generation string
}

/*
Restore 从远端恢复数据库到 opt.OutputPath。

灾备恢复入口：原库可能已不存在，因此无需创建 replica，直接传 handler。
流程：handler.Restore 把远端文件全部拉到临时目录 → 最新快照解压为数据库
文件 → 按 index 顺序把 WAL 段解压拼接为 staging WAL → 逐 index 执行
truncate checkpoint 应用到数据库 → 重命名收尾。无段时走 snapshot-only
直达路径。v1 只恢复最新代的最新快照，不做时间点恢复。
*/
func Restore(ctx context.Context, h Handler, opt RestoreOptions) error {
	return restoreDB(ctx, h, nil, opt)
}

/*
Restore replica 版本：恢复目标文件的 mode/uid/gid 跟随被同步库（fileInfo）。
运行中的 replica 可用它恢复到其他路径。
*/
func (r *replica) Restore(ctx context.Context, opt RestoreOptions) error {
	return restoreDB(ctx, r.handler, r.fileInfo, opt)
}

/*
restoreDB 恢复主流程。fileInfo 用于恢复目标文件的权限匹配，可为 nil。
*/
func restoreDB(ctx context.Context, h Handler, fileInfo os.FileInfo, opt RestoreOptions) (err error) {
	if opt.OutputPath == "" {
		return fmt.Errorf("output path required")
	}

	// 目标路径必须不存在。
	if _, err := os.Stat(opt.OutputPath); err == nil {
		return fmt.Errorf("cannot restore, output path already exists: %s", opt.OutputPath)
	} else if !os.IsNotExist(err) {
		return err
	}

	// 远端文件全部拉到临时目录，保持相对布局。
	tmpdir, err := os.MkdirTemp("", "zsf-sqlitereplica-restore-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpdir) }()

	generation, err := h.Restore(ctx, opt.Generation, tmpdir)
	if err != nil {
		return fmt.Errorf("restore from handler: %w", err)
	}
	genDir := filepath.Join(tmpdir, generation)

	// 解析快照，取 index 最大（最新）的。
	snapshotIndex, err := maxSnapshotIndex(genDir)
	if err != nil {
		return err
	}

	// 解压快照到临时数据库文件。
	tmpPath := opt.OutputPath + ".tmp"
	logger.Logger.Info().Str("generation", generation).Int("index", snapshotIndex).Str("path", tmpPath).Msg("restoring snapshot")
	if err := decompressSnapshot(filepath.Join(genDir, "snapshots", formatSnapshotPath(snapshotIndex)), tmpPath, fileInfo); err != nil {
		return fmt.Errorf("cannot restore snapshot: %w", err)
	}

	// 解析 WAL 段，构造 index → 按 offset 排序的段列表。
	segments, err := walSegmentMap(genDir)
	if err != nil {
		return err
	}

	// 无段：快照直达，重命名收尾。
	if len(segments) == 0 {
		logger.Logger.Info().Msg("snapshot only, finalizing database")
		return os.Rename(tmpPath, opt.OutputPath)
	}

	// 校验段链：从快照 index 起每个 index 都必须有段。
	maxWALIndex := -1
	for index := range segments {
		if index > maxWALIndex {
			maxWALIndex = index
		}
	}
	for index := snapshotIndex; index <= maxWALIndex; index++ {
		if len(segments[index]) == 0 {
			return fmt.Errorf("missing WAL index: %s/%08x", generation, index)
		}
	}

	logger.Logger.Info().
		Str("generation", generation).
		Int("index_min", snapshotIndex).
		Int("index_max", maxWALIndex).
		Msg("restoring wal files")

	// 按 index 顺序：解压拼接为 staging WAL → truncate checkpoint 应用。
	for index := snapshotIndex; index <= maxWALIndex; index++ {
		if err := decompressWAL(genDir, generation, index, segments[index], tmpPath, fileInfo); err != nil {
			return fmt.Errorf("cannot download wal %s/%08x: %w", generation, index, err)
		}
		if err := applyWAL(ctx, index, tmpPath); err != nil {
			return fmt.Errorf("cannot apply wal: %w", err)
		}
	}

	// 复制到最终位置。
	logger.Logger.Info().Msg("renaming database from temporary location")
	if err := os.Rename(tmpPath, opt.OutputPath); err != nil {
		return err
	}
	return nil
}

/*
maxSnapshotIndex 返回快照目录中 index 最大的快照，无快照返回 ErrNoSnapshots。
*/
func maxSnapshotIndex(genDir string) (int, error) {
	dir := filepath.Join(genDir, "snapshots")
	fis, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0, ErrNoSnapshots
	} else if err != nil {
		return 0, err
	}

	index := -1
	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}
		if idx, err := parseSnapshotPath(fi.Name()); err != nil {
			continue // 跳过非法文件名
		} else if idx > index {
			index = idx
		}
	}
	if index == -1 {
		return 0, ErrNoSnapshots
	}
	return index, nil
}

/*
decompressSnapshot 把 LZ4 压缩的快照解压到 filename。fileInfo 为 nil 时用
默认权限。
*/
func decompressSnapshot(srcFilename, dstFilename string, fileInfo os.FileInfo) error {
	if err := mkdirAll(filepath.Dir(dstFilename), nil); err != nil {
		return err
	}

	f, err := createFile(dstFilename, fileInfo)
	if err != nil {
		return err
	}

	src, err := os.Open(srcFilename)
	if err != nil {
		_ = f.Close()
		return err
	}
	defer func() { _ = src.Close() }()

	if _, err := io.Copy(f, lz4.NewReader(src)); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

/*
walSegmentMap 解析段目录，返回 index → 按 offset 排序的段列表，并校验
offset 链：每个 index 首段 offset 必须为 0，后续 offset 严格递增。
*/
func walSegmentMap(genDir string) (map[int][]int64, error) {
	dir := filepath.Join(genDir, "wal")
	fis, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil // 无段目录，snapshot-only
	} else if err != nil {
		return nil, err
	}

	m := make(map[int][]int64)
	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}
		index, offset, err := parseWALSegmentPath(fi.Name())
		if err != nil {
			continue // 跳过非法文件名
		}

		offsets := m[index]
		if len(offsets) == 0 && offset != 0 {
			return nil, fmt.Errorf("missing initial wal segment: index=%08x offset=%d", index, offset)
		} else if len(offsets) > 0 && offsets[len(offsets)-1] >= offset {
			return nil, fmt.Errorf("wal segments out of order: index=%08x offsets=(%d,%d)", index, offsets[len(offsets)-1], offset)
		}
		m[index] = append(offsets, offset)
	}
	for _, offsets := range m {
		sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	}
	return m, nil
}

/*
decompressWAL 把 index 的段按 offset 顺序解压拼接为 <dbPath>-<index>-wal
staging 文件，供 applyWAL 使用。
*/
func decompressWAL(genDir, generation string, index int, offsets []int64, dbPath string, fileInfo os.FileInfo) error {
	f, err := createFile(fmt.Sprintf("%s-%08x-wal", dbPath, index), fileInfo)
	if err != nil {
		return err
	}

	for _, offset := range offsets {
		srcFilename := filepath.Join(genDir, "wal", formatWALSegmentPath(index, offset))
		src, err := os.Open(srcFilename)
		if err != nil {
			_ = f.Close()
			return fmt.Errorf("open segment: %w", err)
		}
		if _, err := io.Copy(f, lz4.NewReader(src)); err != nil {
			_ = src.Close()
			_ = f.Close()
			return fmt.Errorf("decompress segment %s/%08x_%08x: %w", generation, index, offset, err)
		} else if err := src.Close(); err != nil {
			_ = f.Close()
			return err
		}
	}

	return f.Close()
}

/*
applyWAL 把 staging WAL 文件改名为真正的 "-wal" 文件后执行 truncate
checkpoint，把 WAL 内容合并进数据库文件。

使用组件自定义 driver：checkpoint 需要重建 WAL 索引结构，PERSIST_WAL
保证 checkpoint 后 WAL 文件状态可控。
*/
func applyWAL(ctx context.Context, index int, dbPath string) error {
	// staging 文件改为 "-wal" 位置。
	if err := os.Rename(fmt.Sprintf("%s-%08x-wal", dbPath, index), dbPath+"-wal"); err != nil {
		return err
	}

	d, err := sql.Open(driverName, dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()

	var row [3]int
	if err := d.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE);`).Scan(&row[0], &row[1], &row[2]); err != nil {
		return err
	} else if row[0] != 0 {
		return fmt.Errorf("truncation checkpoint failed during restore (%d,%d,%d)", row[0], row[1], row[2])
	}
	return d.Close()
}
