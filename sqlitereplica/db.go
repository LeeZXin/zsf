package sqlitereplica

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/LeeZXin/zsf/logger"
)

// defaultMonitorInterval 默认监听（Sync）间隔。
const defaultMonitorInterval = 1 * time.Second

// defaultCheckpointInterval 默认 checkpoint 间隔：库长期无写入时也会触发一次
// passive checkpoint，让 WAL 段更细粒度。
const defaultCheckpointInterval = 1 * time.Minute

// 默认 checkpoint 触发阈值（WAL 页数）。
const (
	defaultMinCheckpointPageN = 1000
	defaultMaxCheckpointPageN = 10000
	defaultTruncatePageN      = 500000
)

/*
WALPath 返回数据库的 WAL 文件路径。
*/
func (r *replica) WALPath() string {
	return r.path + "-wal"
}

/*
generationNamePath 返回当前 generation 名文件路径。
*/
func (r *replica) generationNamePath() string {
	return filepath.Join(r.metaPath, "generation")
}

/*
generationDir 返回本地 generation 目录。
*/
func (r *replica) generationDir(generation string) string {
	return filepath.Join(r.metaPath, "generations", generation)
}

/*
shadowWALDir 返回本地 shadow WAL 目录。
*/
func (r *replica) shadowWALDir(generation string) string {
	return filepath.Join(r.generationDir(generation), "wal")
}

/*
shadowWALPath 返回本地 shadow WAL 文件路径（文件名与 litestream v0.3.x 一致）。
*/
func (r *replica) shadowWALPath(generation string, index int) string {
	return filepath.Join(r.shadowWALDir(generation), fmt.Sprintf("%08x.wal", index))
}

/*
currentShadowWALPath 返回当前代最新的 shadow WAL 文件路径。
*/
func (r *replica) currentShadowWALPath(generation string) (string, error) {
	index, _, err := r.currentShadowWALIndex(generation)
	if err != nil {
		return "", err
	}
	return r.shadowWALPath(generation, index), nil
}

/*
currentShadowWALIndex 返回当前代 shadow WAL 的最大 index 与总大小。
*/
func (r *replica) currentShadowWALIndex(generation string) (index int, size int64, err error) {
	fis, err := os.ReadDir(r.shadowWALDir(generation))
	if os.IsNotExist(err) {
		return 0, 0, nil // 该代还没有 shadow wal 文件
	} else if err != nil {
		return 0, 0, err
	}

	for _, fi := range fis {
		if idx, err := parseShadowWALName(fi.Name()); err != nil {
			continue // 跳过非法文件名
		} else if idx > index {
			index = idx
		}
		if fi.IsDir() {
			continue
		}
		if info, err := fi.Info(); err != nil {
			continue
		} else {
			size += info.Size()
		}
	}
	return index, size, nil
}

/*
shadowPos 返回 shadow WAL 当前末尾位置（帧对齐）。
*/
func (r *replica) shadowPos() (walPos, error) {
	generation, err := r.CurrentGeneration()
	if err != nil {
		return walPos{}, err
	} else if generation == "" {
		return walPos{}, nil
	}

	index, _, err := r.currentShadowWALIndex(generation)
	if err != nil {
		return walPos{}, err
	}

	fi, err := os.Stat(r.shadowWALPath(generation, index))
	if os.IsNotExist(err) {
		return walPos{Generation: generation, Index: index}, nil
	} else if err != nil {
		return walPos{}, err
	}

	return walPos{Generation: generation, Index: index, Offset: frameAlign(fi.Size(), r.pageSize)}, nil
}

/*
init 初始化数据库连接与 meta 目录。库文件不存在时静默返回（下轮 Sync 重试）。

步骤：缓存文件信息 → 打开自定义 driver 连接与长句柄 → 开启 WAL 并关闭
autocheckpoint → 建辅助表 → 长读事务 → 读页大小 → 建 meta 目录并清理 tmp
残留 → 校验 shadow 与真实 WAL 头是否一致 → 本地旧文件清理。
*/
func (r *replica) init() (err error) {
	// 已初始化直接返回。
	if r.db != nil {
		return nil
	}

	// 库文件还不存在，等它出现。
	fi, err := os.Stat(r.path)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	r.fileInfo = fi

	if fi, err = os.Stat(filepath.Dir(r.path)); err != nil {
		return err
	}
	r.dirInfo = fi

	// 非 sqlite 文件（含空文件）视为「库尚未就绪」：静默等待，等文件变成
	// 合法 sqlite 后自动开始同步。等待日志由 Sync 的缺失门控统一记录（每
	// 个缺失周期一条），这里不打，避免占位文件期间每轮刷屏。
	if ok, err := hasSQLiteHeader(r.path); err != nil {
		return err
	} else if !ok {
		return nil
	}

	// 注意：路径含 "?" 会破坏 DSN 查询串拼接，不支持（见 doc.go）。
	dsn := r.path + fmt.Sprintf("?_busy_timeout=%d", busyTimeout.Milliseconds())
	if r.db, err = sql.Open(driverName, dsn); err != nil {
		return err
	}

	// 长生命周期文件句柄：配合读事务维持锁的持有。
	if r.f, err = os.Open(r.path); err != nil {
		return fmt.Errorf("open db file descriptor: %w", err)
	}

	// 初始化失败时全部回滚，下轮 Sync 重试。
	defer func() {
		if err != nil {
			_ = r.releaseReadLock()
			_ = r.db.Close()
			_ = r.f.Close()
			r.db, r.f = nil, nil
		}
	}()

	// 开启 WAL 并确认。
	var mode string
	if err := r.db.QueryRow(`PRAGMA journal_mode = wal;`).Scan(&mode); err != nil {
		return err
	} else if mode != "wal" {
		return fmt.Errorf("enable wal failed, mode=%q", mode)
	}

	// 关闭本连接的 autocheckpoint，由组件按阈值自行 checkpoint。
	if _, err := r.db.ExecContext(r.ctx, `PRAGMA wal_autocheckpoint = 0;`); err != nil {
		return fmt.Errorf("disable autocheckpoint: %w", err)
	}

	// 辅助表：_zsf_replica_seq 用于空库时强制写 WAL 与快照期间抢读锁；
	// _zsf_replica_lock 用于 checkpoint 后抢写锁。两者都是组件的内部
	// 基础设施，对使用方库有侵入（见 doc.go）。
	if _, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS _zsf_replica_seq (id INTEGER PRIMARY KEY, seq INTEGER);`); err != nil {
		return fmt.Errorf("create _zsf_replica_seq table: %w", err)
	}
	if _, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS _zsf_replica_lock (id INTEGER);`); err != nil {
		return fmt.Errorf("create _zsf_replica_lock table: %w", err)
	}

	// 长读事务阻止其他连接 checkpoint，保证 shadow 同步期间 WAL 不被重置。
	if err := r.acquireReadLock(); err != nil {
		return fmt.Errorf("acquire read lock: %w", err)
	}

	if err := r.db.QueryRow(`PRAGMA page_size;`).Scan(&r.pageSize); err != nil {
		return fmt.Errorf("read page size: %w", err)
	} else if r.pageSize <= 0 {
		return fmt.Errorf("invalid db page size: %d", r.pageSize)
	}

	if err := mkdirAll(r.metaPath, r.dirInfo); err != nil {
		return err
	}

	// 已有 shadow WAL 时校验头是否匹配。不匹配说明上次运行后 WAL 被外部
	// 重置过，由下轮 Sync 的 verify 走开新代路径。注意：这里必须保留
	// generation 名文件（计数器不回退），否则计数器归零后会与远端旧代
	// 撞名，而 pos 仍指向同名旧代，导致新旧时间线的段混在一起。
	if err := r.verifyHeadersMatch(); err != nil {
		logger.Logger.Warn().Err(err).Msg("init: wal header mismatch since last run, new generation on next sync")
	}

	if err := r.clean(); err != nil {
		return fmt.Errorf("clean: %w", err)
	}

	return nil
}

/*
acquireReadLock 开启长读事务阻止其他连接 checkpoint。
*/
func (r *replica) acquireReadLock() error {
	if r.rtx != nil {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(r.ctx, `SELECT COUNT(1) FROM _zsf_replica_seq;`); err != nil {
		_ = tx.Rollback()
		return err
	}

	r.rtx = tx
	return nil
}

/*
releaseReadLock 回滚长读事务（checkpoint 前必须释放）。
*/
func (r *replica) releaseReadLock() error {
	if r.rtx == nil {
		return nil
	}

	err := r.rtx.Rollback()
	r.rtx = nil
	return err
}

/*
ensureWALExists 保证真实 WAL 存在且至少有一个头：写 _zsf_replica_seq 一笔。
空库（从未有写事务）时 WAL 可能不存在或只有头，后续 syncShadowWAL 依赖它。
*/
func (r *replica) ensureWALExists() error {
	if fi, err := os.Stat(r.WALPath()); err == nil && fi.Size() >= walHeaderSize {
		return nil
	}

	_, err := r.db.Exec(`INSERT INTO _zsf_replica_seq (id, seq) VALUES (1, 1) ON CONFLICT (id) DO UPDATE SET seq = seq + 1`)
	return err
}

/*
syncInfo shadow 同步一轮的中间状态。
*/
type syncInfo struct {
	generation    string    // generation 名
	dbModTime     time.Time // 库文件修改时间（checkpoint 间隔判断用）
	walSize       int64     // 真实 WAL 大小（帧对齐）
	shadowWALPath string    // 当前 shadow WAL 文件路径
	shadowWALSize int64     // 当前 shadow WAL 大小（帧对齐）
	restart       bool      // 真实 WAL 头与 shadow 头不一致但可续传
	reason        string    // 非空表示无法续传，需开新 generation
}

/*
verify 校验真实 WAL 与 shadow WAL 是否还能续传。

reason 非空的场景：无 generation、index 超限、shadow 文件缺失/过短、
shadow 比真实 WAL 长（WAL 被外部截断）、仅头且头不匹配、尾页被覆盖。
头不匹配但有帧时置 restart（同一轮 checkpoint 后开新 shadow 文件续传）。
*/
func (r *replica) verify() (info syncInfo, err error) {
	generation, err := r.CurrentGeneration()
	if err != nil {
		return info, fmt.Errorf("cannot find current generation: %w", err)
	} else if generation == "" {
		info.reason = "no generation exists"
		return info, nil
	}
	info.generation = generation

	// 库文件修改时间，checkpoint 间隔判断用。
	fi, err := os.Stat(r.path)
	if err != nil {
		return info, err
	}
	info.dbModTime = fi.ModTime()

	// 真实 WAL 大小。
	fi, err = os.Stat(r.WALPath())
	if err != nil {
		return info, err
	}
	info.walSize = frameAlign(fi.Size(), r.pageSize)

	// shadow WAL 状态。
	index, _, err := r.currentShadowWALIndex(info.generation)
	if err != nil {
		return info, fmt.Errorf("cannot determine shadow wal index: %w", err)
	} else if index >= maxIndex {
		info.reason = "max index exceeded"
		return info, nil
	}
	info.shadowWALPath = r.shadowWALPath(generation, index)

	fi, err = os.Stat(info.shadowWALPath)
	if os.IsNotExist(err) {
		info.reason = "no shadow wal"
		return info, nil
	} else if err != nil {
		return info, err
	}
	info.shadowWALSize = frameAlign(fi.Size(), r.pageSize)

	// shadow 不足一个头。
	if info.shadowWALSize < walHeaderSize {
		info.reason = "short shadow wal"
		return info, nil
	}

	// shadow 比真实 WAL 长：WAL 被外部截断过，无法定位续传点。
	if info.shadowWALSize > info.walSize {
		info.reason = "wal truncated by another process"
		return info, nil
	}

	// 对比 WAL 头。
	hdr0, err := readWALHeader(r.WALPath())
	if err != nil {
		return info, fmt.Errorf("cannot read wal header: %w", err)
	}
	hdr1, err := readWALHeader(info.shadowWALPath)
	if err != nil {
		return info, fmt.Errorf("cannot read shadow wal header: %w", err)
	}
	info.restart = !bytes.Equal(hdr0, hdr1)

	// shadow 只有头且头不匹配：无帧可续，开新代。
	if info.shadowWALSize == walHeaderSize && info.restart {
		info.reason = "wal header only, mismatched"
		return info, nil
	}

	// 校验 shadow 末帧与真实 WAL 同位置内容一致，不一致说明该帧被覆盖过。
	if info.shadowWALSize > walHeaderSize {
		offset := info.shadowWALSize - int64(r.pageSize+walFrameHeaderSize)
		if buf0, err := readWALFileAt(r.WALPath(), offset, int64(r.pageSize+walFrameHeaderSize)); err != nil {
			return info, fmt.Errorf("cannot read last synced wal page: %w", err)
		} else if buf1, err := readWALFileAt(info.shadowWALPath, offset, int64(r.pageSize+walFrameHeaderSize)); err != nil {
			return info, fmt.Errorf("cannot read last synced shadow wal page: %w", err)
		} else if !bytes.Equal(buf0, buf1) {
			info.reason = "wal overwritten by another process"
			return info, nil
		}
	}

	return info, nil
}

/*
syncShadowWAL 把真实 WAL 增量复制到 shadow WAL；restart 时另起 index+1 新文件。
*/
func (r *replica) syncShadowWAL(info syncInfo) (origSize int64, newSize int64, err error) {
	origSize, newSize, err = r.copyToShadowWAL(info.shadowWALPath)
	if err != nil {
		return origSize, newSize, fmt.Errorf("cannot copy to shadow wal: %w", err)
	} else if !info.restart {
		return origSize, newSize, nil
	}

	// 头不一致（WAL 被 checkpoint 重置过）：旧 shadow 文件到此为止，
	// 新建 index+1 的 shadow 文件，从头复制新 WAL。
	index, err := parseShadowWALName(info.shadowWALPath)
	if err != nil {
		return 0, 0, fmt.Errorf("cannot parse shadow wal filename: %s", info.shadowWALPath)
	}
	newShadowWALPath := r.shadowWALPath(info.generation, index+1)
	newSize, err = r.initShadowWALFile(newShadowWALPath)
	if err != nil {
		return 0, 0, fmt.Errorf("cannot init shadow wal file: name=%s err=%w", newShadowWALPath, err)
	}
	return origSize, newSize, nil
}

/*
initShadowWALFile 校验真实 WAL 头（含自身 checksum）后创建新 shadow 文件，
并把当前可读的帧复制进去。新 generation 与 WAL reset 后都会调用。
*/
func (r *replica) initShadowWALFile(filename string) (int64, error) {
	hdr, err := readWALHeader(r.WALPath())
	if err != nil {
		return 0, fmt.Errorf("read header: %w", err)
	}

	bo, err := headerByteOrder(hdr)
	if err != nil {
		return 0, err
	}

	// 校验头自身 checksum，防止读到半写的 WAL 头。
	s0 := binary.BigEndian.Uint32(hdr[walHeaderChecksumOffset:])
	s1 := binary.BigEndian.Uint32(hdr[walHeaderChecksumOffset+4:])
	if v0, v1 := checksum(bo, 0, 0, hdr[:walHeaderChecksumOffset]); v0 != s0 || v1 != s1 {
		return 0, fmt.Errorf("invalid header checksum: (%x,%x) != (%x,%x)", v0, v1, s0, s1)
	}

	mode := os.FileMode(0600)
	if fi := r.fileInfo; fi != nil {
		mode = fi.Mode()
	}
	if err := mkdirAll(filepath.Dir(filename), r.dirInfo); err != nil {
		return 0, err
	} else if err := os.WriteFile(filename, hdr, mode); err != nil {
		return 0, err
	}
	uid, gid := fileinfo(r.fileInfo)
	_ = os.Chown(filename, uid, gid)

	_, newSize, err := r.copyToShadowWAL(filename)
	if err != nil {
		return 0, fmt.Errorf("cannot copy to new shadow wal: %w", err)
	}
	return newSize, nil
}

/*
copyToShadowWAL 把真实 WAL 的增量帧复制到 shadow WAL 文件。

逐帧读取：salt 与头不一致即停（进入未提交/半写区域）；逐帧滚动校验
checksum，不合法即停；只复制到最后一个 commit 帧（帧头 dbSize 非零），
避免把未提交事务复制过去。全部通过后写入临时文件再追加，失败不污染
shadow 文件。
*/
func (r *replica) copyToShadowWAL(filename string) (origWalSize int64, newSize int64, err error) {
	rr, err := os.Open(r.WALPath())
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = rr.Close() }()

	fi, err := rr.Stat()
	if err != nil {
		return 0, 0, err
	}
	origWalSize = frameAlign(fi.Size(), r.pageSize)

	w, err := os.OpenFile(filename, os.O_RDWR, 0666)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = w.Close() }()

	fi, err = w.Stat()
	if err != nil {
		return 0, 0, err
	}
	origSize := frameAlign(fi.Size(), r.pageSize)

	// 读 shadow 头获取 salt 与字节序。
	hdr := make([]byte, walHeaderSize)
	if _, err := io.ReadFull(w, hdr); err != nil {
		return 0, 0, fmt.Errorf("read header: %w", err)
	}
	hsalt0 := binary.BigEndian.Uint32(hdr[16:])
	hsalt1 := binary.BigEndian.Uint32(hdr[20:])

	bo, err := headerByteOrder(hdr)
	if err != nil {
		return 0, 0, err
	}

	// 上次的校验和作为本轮初值，保证跨轮次校验链不断。
	chksum0, chksum1, err := readLastChecksumFrom(w, r.pageSize)
	if err != nil {
		return 0, 0, fmt.Errorf("last checksum: %w", err)
	}

	// 写到临时文件再追加，避免半写污染 shadow 文件。
	tempFilename := filename + ".tmp"
	defer func() { _ = os.Remove(tempFilename) }()

	f, err := createFile(tempFilename, r.fileInfo)
	if err != nil {
		return 0, 0, fmt.Errorf("create temp file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := rr.Seek(origSize, io.SeekStart); err != nil {
		return 0, 0, fmt.Errorf("real wal seek: %w", err)
	} else if _, err := w.Seek(origSize, io.SeekStart); err != nil {
		return 0, 0, fmt.Errorf("shadow wal seek: %w", err)
	}

	// 从上次位置逐帧读取，直到文件尾/半写帧/salt 或校验失败。
	frame := make([]byte, r.pageSize+walFrameHeaderSize)
	offset := origSize
	lastCommitSize := origSize
	for {
		if _, err := io.ReadFull(rr, frame); err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
			break // 正常路径：读到文件尾或半写帧，每轮都会走到，不打日志
		} else if err != nil {
			return 0, 0, fmt.Errorf("read wal: %w", err)
		}

		// 帧 salt 与头不一致：WAL 被 reset 后文件尾部的旧 epoch 残留字节
		//（stale frames），属正常中断路径（库无新写入时每轮都会走到），
		// 静默停止。真实异常由 verify 检测（header 对比 / 尾页对比）。
		salt0 := binary.BigEndian.Uint32(frame[8:])
		salt1 := binary.BigEndian.Uint32(frame[12:])
		if salt0 != hsalt0 || salt1 != hsalt1 {
			break
		}

		// 滚动校验帧 checksum。不合法同上（残留字节/半写帧），静默停止。
		fchksum0 := binary.BigEndian.Uint32(frame[walFrameHeaderChecksumOffset:])
		fchksum1 := binary.BigEndian.Uint32(frame[walFrameHeaderChecksumOffset+4:])
		chksum0, chksum1 = checksum(bo, chksum0, chksum1, frame[:8])  // 帧头
		chksum0, chksum1 = checksum(bo, chksum0, chksum1, frame[24:]) // 帧数据
		if chksum0 != fchksum0 || chksum1 != fchksum1 {
			break
		}

		if _, err := f.Write(frame); err != nil {
			return 0, 0, fmt.Errorf("write temp shadow wal: %w", err)
		}

		offset += int64(len(frame))

		// commit 帧（帧头 dbSize 字段非零）作为可复制的末尾。
		if newDBSize := binary.BigEndian.Uint32(frame[4:]); newDBSize != 0 {
			lastCommitSize = offset
		}
	}

	// 没有新帧直接返回。
	if origSize == lastCommitSize {
		return origSize, lastCommitSize, nil
	}

	walByteN := lastCommitSize - origSize

	// 追加到 shadow WAL 并落盘。
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, 0, fmt.Errorf("temp file seek: %w", err)
	}
	if _, err := io.Copy(w, &io.LimitedReader{R: f, N: walByteN}); err != nil {
		return 0, 0, fmt.Errorf("write shadow file: %w", err)
	}

	if err := f.Close(); err != nil {
		return 0, 0, err
	} else if err := os.Remove(tempFilename); err != nil {
		return 0, 0, err
	}

	if err := w.Sync(); err != nil {
		return 0, 0, err
	} else if err := w.Close(); err != nil {
		return 0, 0, err
	}

	return origWalSize, lastCommitSize, nil
}

/*
checkpoint 对真实 WAL 执行 checkpoint，若 WAL 头因此变化（reset）则另起
index+1 的新 shadow 文件。

chkMu.TryLock 失败说明快照正在进行（快照持锁阻止 checkpoint），本轮跳过。
*/
func (r *replica) checkpoint(ctx context.Context, generation, mode string) error {
	if !r.chkMu.TryLock() {
		return nil
	}
	defer r.chkMu.Unlock()

	shadowWALPath, err := r.currentShadowWALPath(generation)
	if err != nil {
		return err
	}

	// checkpoint 前的 WAL 头，用于判断 checkpoint 是否重置了 WAL。
	hdr, err := readWALHeader(r.WALPath())
	if err != nil {
		return err
	}

	// checkpoint 前先把可读的帧全部复制进 shadow，尽量不丢帧。
	if _, _, err := r.copyToShadowWAL(shadowWALPath); err != nil {
		return fmt.Errorf("cannot copy to end of shadow wal before checkpoint: %w", err)
	}

	// 执行 checkpoint，随后立即写一笔，保证 WAL 有帧（新头可见）。
	if err := r.execCheckpoint(mode); err != nil {
		return err
	} else if _, err = r.db.Exec(`INSERT INTO _zsf_replica_seq (id, seq) VALUES (1, 1) ON CONFLICT (id) DO UPDATE SET seq = seq + 1`); err != nil {
		return err
	}

	// WAL 头未变，本轮结束。
	if other, err := readWALHeader(r.WALPath()); err != nil {
		return err
	} else if bytes.Equal(hdr, other) {
		return nil
	}

	// WAL 被重置：开写事务抢写锁，锁住期间把旧 WAL 尾部复制完，
	// 再为新 WAL 代创建 index+1 的 shadow 文件。
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = rollback(tx) }()

	// 向 lock 表插入一笔把事务提升为写事务（BEGIN 之后再 INSERT 等价于
	// 抢写锁；事务最终回滚，不会产生真实数据）。
	if _, err := tx.ExecContext(ctx, `INSERT INTO _zsf_replica_lock (id) VALUES (1);`); err != nil {
		return fmt.Errorf("_zsf_replica_lock: %w", err)
	}

	if _, _, err := r.copyToShadowWAL(shadowWALPath); err != nil {
		return fmt.Errorf("cannot copy to end of shadow wal: %w", err)
	}

	index, err := parseShadowWALName(shadowWALPath)
	if err != nil {
		return fmt.Errorf("cannot parse shadow wal filename: %s", shadowWALPath)
	}
	newShadowWALPath := r.shadowWALPath(generation, index+1)
	if _, err := r.initShadowWALFile(newShadowWALPath); err != nil {
		return fmt.Errorf("cannot init shadow wal file: name=%s err=%w", newShadowWALPath, err)
	}

	if err := tx.Rollback(); err != nil {
		return fmt.Errorf("rollback post-checkpoint tx: %w", err)
	}
	return nil
}

/*
execCheckpoint 执行 PRAGMA wal_checkpoint(mode)。

长读事务会阻塞 checkpoint，因此先释放，checkpoint 后立刻重新获取。
*/
func (r *replica) execCheckpoint(mode string) (err error) {
	if r.db == nil {
		return nil
	}

	t := time.Now()
	defer func() {
		logger.Logger.Debug().
			Str("mode", mode).
			Dur("elapsed", time.Since(t)).
			Err(err).
			Msg("checkpoint")
	}()

	if err := r.releaseReadLock(); err != nil {
		return fmt.Errorf("release read lock: %w", err)
	}
	defer func() { _ = r.acquireReadLock() }()

	rawsql := `PRAGMA wal_checkpoint(` + mode + `);`
	var row [3]int
	if err := r.db.QueryRow(rawsql).Scan(&row[0], &row[1], &row[2]); err != nil {
		return err
	}
	return nil
}

/*
shadowWALFile 按位置读取 shadow WAL 文件的 reader，跟踪剩余字节与当前位置。
*/
type shadowWALFile struct {
	f   *os.File
	n   int64
	pos walPos
}

// Name 返回底层文件名。
func (rd *shadowWALFile) Name() string { return rd.f.Name() }

// Close 关闭底层文件句柄。
func (rd *shadowWALFile) Close() error { return rd.f.Close() }

// N 返回剩余可读字节数。
func (rd *shadowWALFile) N() int64 { return rd.n }

// walPos 返回当前位置。
func (rd *shadowWALFile) walPos() walPos { return rd.pos }

// Read 读取字节并推进位置，到达可用段末尾返回 io.EOF。
func (rd *shadowWALFile) Read(p []byte) (n int, err error) {
	if rd.n <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > rd.n {
		p = p[0:rd.n]
	}
	n, err = rd.f.Read(p)
	rd.n -= int64(n)
	rd.pos.Offset += int64(n)
	return n, err
}

/*
shadowWALReader 打开指定位置的 shadow WAL 文件。当前位置已到文件尾且下一个
文件存在时自动跳到下一 index（checkpoint 边界处段上传的衔接）。
*/
func (r *replica) shadowWALReader(pos walPos) (rd *shadowWALFile, err error) {
	rd, err = r.shadowWALReaderAt(pos)
	if err != nil {
		return nil, err
	} else if rd.N() > 0 {
		return rd, nil
	} else if err := rd.Close(); err != nil {
		return nil, err
	}

	// 当前文件无剩余数据，尝试下一个 index。
	pos.Index, pos.Offset = pos.Index+1, 0
	rd, err = r.shadowWALReaderAt(pos)
	if os.IsNotExist(err) {
		return nil, io.EOF
	}
	return rd, err
}

/*
shadowWALReaderAt 打开 pos 处的 shadow WAL 文件并 seek 到 offset。
*/
func (r *replica) shadowWALReaderAt(pos walPos) (rd *shadowWALFile, err error) {
	filename := r.shadowWALPath(pos.Generation, pos.Index)

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			_ = f.Close()
		}
	}()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// 帧对齐的文件大小，offset 超出即报错。
	fileSize := frameAlign(fi.Size(), r.pageSize)
	if pos.Offset > fileSize {
		return nil, fmt.Errorf("wal reader offset too high: %d > %d", pos.Offset, fileSize)
	}

	if _, err := f.Seek(pos.Offset, io.SeekStart); err != nil {
		return nil, err
	}

	return &shadowWALFile{
		f:   f,
		n:   fileSize - pos.Offset,
		pos: pos,
	}, nil
}

/*
CurrentGeneration 返回 meta 目录中记录的当前 generation 名，无则空串。
*/
func (r *replica) CurrentGeneration() (string, error) {
	buf, err := os.ReadFile(r.generationNamePath())
	if os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	generation := strings.TrimSpace(string(buf))
	if !IsGenerationName(generation) {
		return "", nil
	}
	return generation, nil
}

/*
createGeneration 新建 generation：generation 目录 + 初始 shadow WAL +
原子写入 generation 名文件，并清理本地旧代目录。

generation 名用零填充 hex 计数器（非 litestream 的随机 hex），保证远端
Restore("") 可用字典序选出最新代。计数器从 generation 名文件读取；文件
缺失/损坏时回退为本地现存 generation 目录的最大名，避免计数器回退与
远端旧代撞名。
*/
func (r *replica) createGeneration() (string, error) {
	n := uint64(0)
	if buf, err := os.ReadFile(r.generationNamePath()); err == nil {
		if v, err := strconv.ParseUint(strings.TrimSpace(string(buf)), 16, 64); err == nil {
			n = v
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	// 名文件缺失或损坏时的兜底：取本地现存 generation 目录的最大名。
	if n == 0 {
		if fis, err := os.ReadDir(filepath.Join(r.metaPath, "generations")); err == nil {
			for _, fi := range fis {
				if !IsGenerationName(fi.Name()) {
					continue
				}
				if v, err := strconv.ParseUint(fi.Name(), 16, 64); err == nil && v > n {
					n = v
				}
			}
		}
	}

	// pos 作为计数器下限：pos 是上次成功上传的 generation。名文件损坏且
	// 本地目录被清理时，保证不回退到远端已存在的代名。
	if v, err := strconv.ParseUint(r.pos.Generation, 16, 64); err == nil && v > n {
		n = v
	}

	if n == math.MaxUint64 {
		return "", fmt.Errorf("generation counter overflow")
	}
	n++
	generation := fmt.Sprintf("%016x", n)

	// 建目录并初始化 shadow WAL（复制真实 WAL 头 + 当前可读帧）。
	dir := r.generationDir(generation)
	if err := mkdirAll(dir, r.dirInfo); err != nil {
		return "", err
	}
	if _, err := r.initShadowWALFile(r.shadowWALPath(generation, 0)); err != nil {
		return "", fmt.Errorf("initialize shadow wal: %w", err)
	}

	// 原子写入 generation 名文件。
	generationNamePath := r.generationNamePath()
	mode := os.FileMode(0600)
	if r.fileInfo != nil {
		mode = r.fileInfo.Mode()
	}
	if err := os.WriteFile(generationNamePath+".tmp", []byte(generation+"\n"), mode); err != nil {
		return "", fmt.Errorf("write generation temp file: %w", err)
	}
	uid, gid := fileinfo(r.fileInfo)
	_ = os.Chown(generationNamePath+".tmp", uid, gid)
	if err := os.Rename(generationNamePath+".tmp", generationNamePath); err != nil {
		return "", fmt.Errorf("rename generation file: %w", err)
	}

	// 清理本地旧代目录。
	if err := r.clean(); err != nil {
		return "", err
	}

	return generation, nil
}

/*
verifyHeadersMatch 校验真实 WAL 头与最新 shadow WAL 头一致，用于 init 时
判断上次运行位置是否仍有效。
*/
func (r *replica) verifyHeadersMatch() error {
	generation, err := r.CurrentGeneration()
	if err != nil {
		return err
	} else if generation == "" {
		return nil
	}

	shadowWALPath, err := r.currentShadowWALPath(generation)
	if err != nil {
		return fmt.Errorf("cannot determine current shadow wal path: %w", err)
	}

	hdr0, err := readWALHeader(r.WALPath())
	if os.IsNotExist(err) {
		return fmt.Errorf("no primary wal: %w", err)
	} else if err != nil {
		return fmt.Errorf("primary wal header: %w", err)
	}

	hdr1, err := readWALHeader(shadowWALPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("no shadow wal")
	} else if err != nil {
		return fmt.Errorf("shadow wal header: %w", err)
	}

	if !bytes.Equal(hdr0, hdr1) {
		return fmt.Errorf("wal header mismatch %x <> %x on %s", hdr0, hdr1, shadowWALPath)
	}
	return nil
}

/*
clean 清理本地 meta：非当前代的 generation 目录 + 已同步的 shadow WAL 文件。
*/
func (r *replica) clean() error {
	if err := r.cleanGenerations(); err != nil {
		return err
	}
	return r.cleanWAL()
}

/*
cleanGenerations 删除本地非当前代的 generation 目录。
*/
func (r *replica) cleanGenerations() error {
	generation, err := r.CurrentGeneration()
	if err != nil {
		return err
	}

	dir := filepath.Join(r.metaPath, "generations")
	fis, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	for _, fi := range fis {
		if fi.Name() == generation {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, fi.Name())); err != nil {
			return err
		}
	}
	return nil
}

/*
cleanWAL 删除本地已上传的 shadow WAL 文件。

保留规则：只删 index < pos.Index-1 的文件（多保留一个）。这保证重启后
pos 指向的文件一定还在本地——shadow 文件只追加不重写，续传依赖它。
*/
func (r *replica) cleanWAL() error {
	generation, err := r.CurrentGeneration()
	if err != nil {
		return err
	}

	pos := r.pos
	if pos.Generation != generation {
		return nil // 代不同，不清理（旧代目录由 cleanGenerations 处理）
	}

	if pos.Index <= 0 {
		return nil
	}
	minIdx := pos.Index - 1 // 多保留一个文件

	dir := r.shadowWALDir(generation)
	fis, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	for _, fi := range fis {
		idx, err := parseShadowWALName(fi.Name())
		if err != nil || idx >= minIdx {
			continue
		}
		if err := os.Remove(filepath.Join(dir, fi.Name())); err != nil {
			return err
		}
	}
	return nil
}

/*
parseShadowWALName 解析 shadow WAL 文件名 "%08x.wal"（先取 basename），
返回 index。
*/
func parseShadowWALName(s string) (int, error) {
	s = filepath.Base(s)
	if len(s) != len("00000000.wal") || s[len(s)-4:] != ".wal" {
		return 0, fmt.Errorf("invalid wal path: %s", s)
	}
	i64, err := strconv.ParseUint(s[:8], 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid wal path: %s", s)
	}
	return int(i64), nil
}
