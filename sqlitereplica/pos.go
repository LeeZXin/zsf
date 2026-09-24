package sqlitereplica

import (
	"fmt"
	"regexp"
	"strconv"
)

// 远端文件名后缀（与 litestream v0.3.x 一致）。
const (
	// walSegmentExt WAL 段文件后缀。
	walSegmentExt = ".wal.lz4"
	// snapshotExt 快照文件后缀。
	snapshotExt = ".snapshot.lz4"
)

/*
walPos 表示 shadow WAL 中的一个位置：generation + WAL 文件 index + 文件内 offset。

同时用作「已上传到远端」的进度标记（见 replica.walPos），并持久化在
metaPath/replica-pos 中，供进程重启后恢复上传位置。
*/
type walPos struct {
	Generation string // generation 名
	Index      int    // WAL 文件 index
	Offset     int64  // 文件内 offset（帧对齐）
}

// String 返回位置的可读表示，零值返回空串。
func (p walPos) String() string {
	if p.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s/%08x:%d", p.Generation, p.Index, p.Offset)
}

// IsZero 返回 p 是否为零值。
func (p walPos) IsZero() bool {
	return p == (walPos{})
}

/*
snapshotInfo 快照元数据。
*/
type snapshotInfo struct {
	Generation string // generation 名
	Index      int    // 快照对应的 shadow WAL index（恢复起点）
	Size       int64  // 压缩后大小
}

// snapshotPath 返回快照在远端的相对路径。
func snapshotPath(generation string, index int) string {
	return generation + "/snapshots/" + formatSnapshotPath(index)
}

// walSegmentPath 返回 WAL 段在远端的相对路径。
func walSegmentPath(generation string, index int, offset int64) string {
	return generation + "/wal/" + formatWALSegmentPath(index, offset)
}

// formatSnapshotPath 格式化快照文件名。
func formatSnapshotPath(index int) string {
	return fmt.Sprintf("%08x%s", index, snapshotExt)
}

var snapshotPathRegex = regexp.MustCompile(`^([0-9a-f]{8})\.snapshot\.lz4$`)

// parseSnapshotPath 解析快照文件名（仅取 basename），返回 index。
func parseSnapshotPath(s string) (index int, err error) {
	a := snapshotPathRegex.FindStringSubmatch(s)
	if a == nil {
		return 0, fmt.Errorf("invalid snapshot path: %s", s)
	}
	i64, _ := strconv.ParseUint(a[1], 16, 64)
	return int(i64), nil
}

// formatWALSegmentPath 格式化 WAL 段文件名。
func formatWALSegmentPath(index int, offset int64) string {
	return fmt.Sprintf("%08x_%08x%s", index, offset, walSegmentExt)
}

var walSegmentPathRegex = regexp.MustCompile(`^([0-9a-f]{8})_([0-9a-f]{8})\.wal\.lz4$`)

// parseWALSegmentPath 解析 WAL 段文件名（仅取 basename），返回 index 与 offset。
func parseWALSegmentPath(s string) (index int, offset int64, err error) {
	a := walSegmentPathRegex.FindStringSubmatch(s)
	if a == nil {
		return 0, 0, fmt.Errorf("invalid wal segment path: %s", s)
	}
	i64, _ := strconv.ParseUint(a[1], 16, 64)
	off64, _ := strconv.ParseUint(a[2], 16, 64)
	return int(i64), int64(off64), nil
}
