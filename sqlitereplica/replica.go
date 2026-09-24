package sqlitereplica

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/bytedance/sonic"

	"github.com/pierrec/lz4/v4"
)

// defaultSnapshotInterval 默认快照间隔。
const defaultSnapshotInterval = 1 * time.Hour

// finalSyncTimeout Close 时最终同步的超时。
const finalSyncTimeout = 1 * time.Minute

/*
SyncOptions replica 组件配置。Path 与 Handler 必填，其余零值取默认。
*/
type SyncOptions struct {
	// Path 被同步的 SQLite 数据库文件路径。
	Path string
	// Handler 远端存储实现（见 file 与 oss 子包）。
	Handler Handler
	// MetaPath shadow WAL、generation、位置等元数据目录。
	// 默认 <dir>/.<basename>-litestream。
	MetaPath string
	// MonitorInterval Sync 间隔，默认 1s。
	MonitorInterval time.Duration
	// CheckpointInterval 库长期无写入时触发 passive checkpoint 的间隔，默认 1m。
	CheckpointInterval time.Duration
	// MinCheckpointPageN WAL 超过该页数触发 passive checkpoint，默认 1000。
	MinCheckpointPageN int
	// MaxCheckpointPageN WAL 超过该页数触发 restart checkpoint（阻塞新事务），默认 10000。
	MaxCheckpointPageN int
	// TruncatePageN WAL 超过该页数触发 truncate checkpoint，默认 500000。
	TruncatePageN int
	// SnapshotInterval 快照间隔，默认 1h；<=0 时不周期快照（仅新代时快照）。
	SnapshotInterval time.Duration
}

/*
replica SQLite 复制组件：监听数据库文件的 WAL 变更，持续将「快照 + WAL 段」
同步到远端存储，并支持从远端恢复数据库文件。

并发模型：Sync/Snapshot 由 r.mu 串行化；checkpoint 与快照由 chkMu 互斥
（快照期间 checkpoint 自动跳过）。
*/
type replica struct {
	mu    sync.Mutex // 串行化 Sync/Snapshot/Checkpoint
	chkMu sync.Mutex // checkpoint 锁，快照期间阻止 checkpoint

	path     string  // 数据库文件路径
	metaPath string  // 元数据目录
	handler  Handler // 远端存储

	db       *sql.DB  // zsf-sqlitereplica driver 连接
	f        *os.File // 长生命周期 db 文件句柄
	rtx      *sql.Tx  // 长读事务，阻止他人 checkpoint
	pageSize int

	fileInfo os.FileInfo // init 时缓存的 db 文件信息
	dirInfo  os.FileInfo // init 时缓存的父目录信息

	// 配置（New 中已填默认值）
	monitorInterval    time.Duration
	checkpointInterval time.Duration
	minCheckpointPageN int
	maxCheckpointPageN int
	truncatePageN      int
	snapshotInterval   time.Duration

	pos       walPos // 已上传位置
	posLoaded bool   // pos 是否已从磁盘加载

	missingLogged bool // 库缺失期间是否已记过等待日志（防每秒刷屏）

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

/*
Sync 创建并启动 SQLite 同步（一行接入，同步侧唯一公开入口）。

内部完成：参数校验（错误直接 Fatal，fast fail）→ 创建组件 → 启动监听
goroutine → 注册进程退出钩子（quit 最终钩子，保证 replica 在业务连接
关闭之后才关闭，WAL 得以保留、重启后增量续传而非全量）。

例：

	sqlitereplica.Sync(sqlitereplica.SyncOptions{
		Path:    "data/sqlite3.db",
		Handler: file.NewHandler("backup-dir"),
	})
*/
func Sync(opts SyncOptions) {
	r := newReplica(opts)
	quit.AddFinalShutdownHook(r.Close)
	go func() {
		if err := r.Run(); err != nil {
			if !errors.Is(err, context.Canceled) {
				logger.Logger.Fatal().Err(err).Msg("sqlitereplica: cannot run replica")
			}
		}
	}()
}

/*
newReplica 校验参数并创建 replica（不启动、不注册钩子）。测试内部使用。
*/
func newReplica(opts SyncOptions) *replica {
	if opts.Path == "" {
		logger.Logger.Fatal().Msg("sqlitereplica: db path required")
	}
	if opts.Handler == nil {
		logger.Logger.Fatal().Msg("sqlitereplica: handler required")
	}
	// db 文件必须已存在：replica 的契约是「库已就绪后才创建组件」，启动
	// 顺序错误时直接 Fatal（fast fail），而不是运行期每秒刷等待日志。
	// 运行期库被删除/替换仍会静默等待重建。灾备恢复请用包级 Restore
	// 函数（原库可能已不存在，无需 replica）。
	if fi, err := os.Stat(opts.Path); os.IsNotExist(err) {
		logger.Logger.Fatal().Str("path", opts.Path).Msg("sqlitereplica: db file does not exist")
	} else if err != nil {
		logger.Logger.Fatal().Err(err).Str("path", opts.Path).Msg("sqlitereplica: cannot stat db file")
	} else if fi.IsDir() {
		logger.Logger.Fatal().Str("path", opts.Path).Msg("sqlitereplica: db path is a directory")
	}
	if opts.MetaPath == "" {
		dir, file := filepath.Split(opts.Path)
		opts.MetaPath = filepath.Join(dir, "."+file+"-litestream")
	}
	if opts.MonitorInterval == 0 {
		opts.MonitorInterval = defaultMonitorInterval
	}
	if opts.CheckpointInterval == 0 {
		opts.CheckpointInterval = defaultCheckpointInterval
	}
	if opts.MinCheckpointPageN == 0 {
		opts.MinCheckpointPageN = defaultMinCheckpointPageN
	}
	if opts.MaxCheckpointPageN == 0 {
		opts.MaxCheckpointPageN = defaultMaxCheckpointPageN
	}
	if opts.TruncatePageN == 0 {
		opts.TruncatePageN = defaultTruncatePageN
	}
	if opts.SnapshotInterval == 0 {
		opts.SnapshotInterval = defaultSnapshotInterval
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &replica{
		path:               opts.Path,
		metaPath:           opts.MetaPath,
		handler:            opts.Handler,
		monitorInterval:    opts.MonitorInterval,
		checkpointInterval: opts.CheckpointInterval,
		minCheckpointPageN: opts.MinCheckpointPageN,
		maxCheckpointPageN: opts.MaxCheckpointPageN,
		truncatePageN:      opts.TruncatePageN,
		snapshotInterval:   opts.SnapshotInterval,
		ctx:                ctx,
		cancel:             cancel,
	}
}

/*
Run 启动监听循环并阻塞，直到 Close 被调用。

循环内容：按 MonitorInterval 执行 Sync，按 SnapshotInterval 执行 Snapshot。
退出前执行最终同步并释放数据库资源。
*/
func (r *replica) Run() error {
	// 清理上次崩溃遗留的 tmp 文件。
	if err := removeTmpFiles(r.metaPath); err != nil {
		logger.Logger.Warn().Err(err).Msg("cannot remove tmp files")
	}

	monTicker := time.NewTicker(r.monitorInterval)
	defer monTicker.Stop()

	var snapC <-chan time.Time
	if r.snapshotInterval > 0 {
		snapTicker := time.NewTicker(r.snapshotInterval)
		defer snapTicker.Stop()
		snapC = snapTicker.C
	}
	logger.Logger.Info().Msgf("start sync sqlite: %s", r.path)
	ctx := context.Background()
	for {
		select {
		case <-r.ctx.Done():
			r.shutdown()
			return nil
		case <-monTicker.C:
			// Close 已执行时跳过本轮：shutdown 已经关闭并置空了数据库
			// 连接，再跑 Sync 会重新打开连接造成泄漏。
			if r.ctx.Err() != nil {
				continue
			}
			if err := r.Sync(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Logger.Error().Err(err).Msg("sync error")
			}
		case <-snapC:
			if r.ctx.Err() != nil {
				continue
			}
			if _, err := r.Snapshot(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrNoGeneration) {
				logger.Logger.Error().Err(err).Msg("snapshot error")
			}
		}
	}
}

/*
Close 幂等关闭组件：取消内部 ctx，执行最终同步（独立超时 ctx），释放
读锁与数据库连接。可能阻塞至最终同步完成（最长 1 分钟）。
*/
func (r *replica) Close() {
	if r.cancel != nil {
		r.cancel()
		r.shutdown()
	}
}

/*
shutdown 最终同步与资源释放，closeOnce 保证只执行一次。
*/
func (r *replica) shutdown() {
	r.closeOnce.Do(func() {
		// 外层 ctx 已取消，最终同步用独立超时 ctx 兜底。
		ctx, cancel := context.WithTimeout(context.Background(), finalSyncTimeout)
		defer cancel()
		if err := r.Sync(ctx); err != nil {
			logger.Logger.Warn().Err(err).Msg("final sync failed")
		}

		_ = r.releaseReadLock()
		if r.db != nil {
			_ = r.db.Close()
			r.db = nil
		}
		if r.f != nil {
			_ = r.f.Close()
			r.f = nil
		}
	})
}

/*
Sync 手动执行一轮同步（monitor 循环与 Close 最终同步共用）：

阶段一 shadow 同步：init → 保证 WAL 存在 → 校验续传状态（必要时开新
generation）→ 逐帧复制真实 WAL 到 shadow → 按阈值 checkpoint → 本地清理。
阶段二远端同步：新代/pos 丢失时先快照（先清空该代远端残留），否则按
标记判断是否补快照，随后把 shadow WAL 增量压缩上传并持久化 pos。

库文件不存在时静默返回（下轮重试）。
*/
func (r *replica) Sync(ctx context.Context) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 先加载持久化的 pos：阶段一的 cleanWAL 依赖它保留续传所需的 shadow
	// 文件，createGeneration 依赖它作为计数器下限（防止 generation 撞名）。
	r.loadPosOnce()

	// 阶段一：shadow 同步。
	if err := r.init(); err != nil {
		return err
	} else if r.db == nil {
		// 运行期库被删除或尚未就绪（占位文件）：静默等待，每个缺失周期
		// 只记一条 Debug，避免每秒刷屏。库重建后自动恢复同步。
		if !r.missingLogged {
			r.missingLogged = true
			logger.Logger.Debug().Str("path", r.path).Msg("sync: database not available, waiting")
		}
		return nil
	}
	r.missingLogged = false

	if err := r.ensureWALExists(); err != nil {
		return fmt.Errorf("ensure wal exists: %w", err)
	}

	info, err := r.verify()
	if err != nil {
		return fmt.Errorf("cannot verify wal state: %w", err)
	}

	// 无法续传：开新 generation。
	if info.reason != "" {
		if info.generation, err = r.createGeneration(); err != nil {
			return fmt.Errorf("create generation: %w", err)
		}
		logger.Logger.Info().Str("generation", info.generation).Str("reason", info.reason).Msg("sync: new generation")

		info.shadowWALPath = r.shadowWALPath(info.generation, 0)
		info.shadowWALSize = walHeaderSize
		info.restart = false
		info.reason = ""
	}

	origWALSize, newWALSize, err := r.syncShadowWAL(info)
	if err != nil {
		return fmt.Errorf("sync shadow wal: %w", err)
	}

	// checkpoint 阈值判断（truncate > restart > passive > 间隔兜底）。
	var checkpoint bool
	checkpointMode := checkpointModePassive
	if r.truncatePageN > 0 && origWALSize >= calcWALSize(r.pageSize, r.truncatePageN) {
		checkpoint, checkpointMode = true, checkpointModeTruncate
	} else if r.maxCheckpointPageN > 0 && newWALSize >= calcWALSize(r.pageSize, r.maxCheckpointPageN) {
		checkpoint, checkpointMode = true, checkpointModeRestart
	} else if newWALSize >= calcWALSize(r.pageSize, r.minCheckpointPageN) {
		checkpoint = true
	} else if r.checkpointInterval > 0 && !info.dbModTime.IsZero() && time.Since(info.dbModTime) > r.checkpointInterval && newWALSize > calcWALSize(r.pageSize, 1) {
		checkpoint = true
	}

	if checkpoint {
		if err := r.checkpoint(ctx, info.generation, checkpointMode); err != nil {
			return fmt.Errorf("checkpoint: mode=%v err=%w", checkpointMode, err)
		}
	}

	if err := r.clean(); err != nil {
		return fmt.Errorf("cannot clean: %w", err)
	}

	// 阶段二：远端同步。
	if err := r.syncReplica(ctx); err != nil {
		return fmt.Errorf("replica sync: %w", err)
	}

	return nil
}

/*
syncReplica 阶段二：远端同步。必须在持有 r.mu 时调用。
*/
func (r *replica) syncReplica(ctx context.Context) (err error) {
	dpos, err := r.shadowPos()
	if err != nil {
		return fmt.Errorf("cannot determine current position: %w", err)
	} else if dpos.IsZero() {
		return fmt.Errorf("no generation, waiting for data")
	}
	generation := dpos.Generation

	if r.pos.IsZero() || r.pos.Generation != generation {
		// 新代首次同步（全新启动、WAL 重置开新代、或 pos 文件丢失）：
		// 先清空该代远端残留再上传快照，成功后才推进 pos。清空是为了
		// 计数器 generation 名在异常回退时可能与远端旧代撞名，避免新旧
		// 时间线的段混在一起。
		if err := r.handler.Remove(ctx, generation); err != nil {
			return fmt.Errorf("remove generation %q before initial sync: %w", generation, err)
		}

		info, err := r.snapshot(ctx)
		if err != nil {
			return err
		}
		if info.Generation != generation {
			return fmt.Errorf("generation changed during snapshot, exiting sync")
		}

		// 旧代远端清理（best-effort）：counter 名不会复用，残留只占空间，
		// 不影响恢复（恢复取名字最大的代）。pos 有效时清理紧邻的上一代；
		// pos 丢失（零值）时无法定位上一代，按计数器顺序清理所有更小的代
		//（计数器只递增，旧代名必然小于当前代）。
		if old := r.pos; old.Generation != "" && old.Generation != generation {
			r.removeRemoteGeneration(ctx, old.Generation)
		} else if old.Generation == "" {
			if n, err := strconv.ParseUint(generation, 16, 64); err == nil {
				for i := uint64(1); i < n; i++ {
					r.removeRemoteGeneration(ctx, fmt.Sprintf("%016x", i))
				}
			}
		}

		r.pos = walPos{Generation: info.Generation, Index: info.Index}
	} else if !r.hasSnapshotMarker(generation) {
		// 快照标记缺失（快照上传后、标记写盘前崩溃）：补一次快照，幂等。
		if _, err := r.snapshot(ctx); err != nil {
			return err
		}
	}

	// 上传 shadow WAL 增量直到追平。
	for {
		if err := r.uploadWALSegment(ctx); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}
	}

	return r.savePos()
}

/*
uploadWALSegment 从 r.pos 读 shadow WAL，LZ4 压缩后作为一段上传到远端，
成功后推进 r.pos。

段内容：offset 为 0 时以 WAL 头开头，其后是连续的完整帧（salt 必须与头
一致）。段在远端名为 %08x_%08x.wal.lz4（index_offset），恢复时按 offset
排序拼接回完整 WAL。
*/
func (r *replica) uploadWALSegment(ctx context.Context) (err error) {
	rd, err := r.shadowWALReader(r.pos)
	if errors.Is(err, io.EOF) {
		return io.EOF
	} else if err != nil {
		return fmt.Errorf("shadow wal reader: %w", err)
	}

	pos := rd.walPos()
	startTime := time.Now()

	logger.Logger.Debug().Str("position", pos.String()).Msg("write wal segment")

	// 写侧（shadow 读取 + LZ4 压缩）放独立 goroutine：handler.Output 中途
	// 失败不再读 pipe 时，主流程可用 CloseWithError 解阻塞写侧，避免卡死
	// 在 pipe 写。
	pr, pw := io.Pipe()

	var (
		feedErr      error
		newPos       walPos
		bytesWritten int64
	)
	feedDone := make(chan struct{})
	go func() {
		defer close(feedDone)
		defer func() { _ = rd.Close() }()

		zw := lz4.NewWriter(pw)
		defer func() {
			if cerr := zw.Close(); cerr != nil && feedErr == nil {
				feedErr = cerr
			}
			// 失败时把错误传给读侧：handler 读到错误必须放弃本次写入，
			// 否则会把截断数据当作完整段持久化（见 Handler 接口约定）。
			if feedErr != nil {
				_ = pw.CloseWithError(feedErr)
			} else {
				_ = pw.Close()
			}
		}()

		// 段首 offset 为 0 时先复制 WAL 头。
		var psalt uint64
		if rd.walPos().Offset == 0 {
			buf := make([]byte, walHeaderSize)
			if _, err := io.ReadFull(rd, buf); err != nil {
				feedErr = err
				return
			}

			psalt = binary.BigEndian.Uint64(buf[16:24])

			if _, err := zw.Write(buf); err != nil {
				feedErr = err
				return
			}
			bytesWritten += int64(len(buf))
		}

		// 逐帧复制，salt 必须与头一致（跨段连续性由 shadow 文件边界保证）。
		for {
			buf := make([]byte, walFrameHeaderSize+r.pageSize)
			if _, err := io.ReadFull(rd, buf); err == io.EOF {
				break
			} else if err != nil {
				feedErr = err
				return
			}

			salt := binary.BigEndian.Uint64(buf[8:16])
			if psalt != 0 && psalt != salt {
				feedErr = fmt.Errorf("replica salt mismatch: %s", pos.String())
				return
			}
			psalt = salt

			if _, err := zw.Write(buf); err != nil {
				feedErr = err
				return
			}
			bytesWritten += int64(len(buf))
		}

		newPos = rd.walPos()
	}()

	// 读侧交给 handler 上传。
	if outputErr := r.handler.Output(ctx, walSegmentPath(pos.Generation, pos.Index, pos.Offset), pr); outputErr != nil {
		_ = pw.CloseWithError(outputErr) // 解阻塞写侧
		<-feedDone
		return fmt.Errorf("handler output: %w", outputErr)
	}

	// 等写侧完成。ctx 兜底：不消费 pipe 的非法 handler 也不会卡死主流程。
	select {
	case <-feedDone:
	case <-ctx.Done():
		_ = pw.CloseWithError(ctx.Err())
		<-feedDone
		return ctx.Err()
	}
	if feedErr != nil {
		return feedErr
	}

	r.pos = newPos
	logger.Logger.Debug().
		Str("position", pos.String()).
		Dur("elapsed", time.Since(startTime)).
		Int64("bytes", bytesWritten).
		Msg("wal segment written")
	return nil
}

/*
Snapshot 执行一次快照：冻结数据库文件（读事务），把整库 LZ4 压缩上传
为 <gen>/snapshots/%08x.snapshot.lz4，快照 index = 当时的 shadow WAL
index（恢复时从该 index 起重放段）。成功后写本地快照标记文件。
*/
func (r *replica) Snapshot(ctx context.Context) (snapshotInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshot(ctx)
}

/*
snapshot 快照实现，必须在持有 r.mu 时调用。

一致性依据：shadow 文件边界由完整的 restart checkpoint 产生（阻塞模式
吸收全部已提交帧），故任意时刻 db 文件包含 shadow 0..N-1 的全部内容；
恢复时「快照 + 从 index N 起的段」完整覆盖最新状态，重复帧由 SQLite 的
salt 机制幂等跳过。
*/
func (r *replica) snapshot(ctx context.Context) (info snapshotInfo, err error) {
	if r.db == nil {
		return info, fmt.Errorf("no database available")
	}

	// 阻止自身 checkpoint 与快照并发。
	r.chkMu.Lock()
	defer r.chkMu.Unlock()

	// 快照前尝试一次 passive checkpoint 刷盘（长读事务持有时是 no-op，无害）。
	if _, err := r.db.ExecContext(ctx, `PRAGMA wal_checkpoint(PASSIVE);`); err != nil {
		return info, fmt.Errorf("pre-snapshot checkpoint: %w", err)
	}

	// 读事务冻结数据库文件：任何 reader 存在期间 checkpoint 都无法进行，
	// 因此拷贝期间 db 文件内容不变。
	tx, err := r.db.Begin()
	if err != nil {
		return info, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT COUNT(1) FROM _zsf_replica_seq;`); err != nil {
		_ = tx.Rollback()
		return info, err
	}
	defer func() { _ = tx.Rollback() }()

	// 快照 index = 当前 shadow index。
	pos, err := r.shadowPos()
	if err != nil {
		return info, fmt.Errorf("cannot determine db position: %w", err)
	} else if pos.IsZero() {
		return info, ErrNoGeneration
	}
	info = snapshotInfo{Generation: pos.Generation, Index: pos.Index}

	if _, err := r.f.Seek(0, io.SeekStart); err != nil {
		return info, err
	}

	// db 文件 → pipe → LZ4 压缩 → handler.Output 流式上传。
	pr, pw := io.Pipe()
	var copyErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		zw := lz4.NewWriter(pw)
		if _, err := io.Copy(zw, r.f); err != nil {
			copyErr = err
			_ = pw.CloseWithError(err)
			return
		} else if err := zw.Close(); err != nil {
			copyErr = err
			_ = pw.CloseWithError(err)
			return
		}
		_ = pw.Close()
	}()

	// 统计压缩后的字节数（handler 实际读走的量）。
	sr := &sizeReader{r: pr}

	startTime := time.Now()
	if err := r.handler.Output(ctx, snapshotPath(pos.Generation, pos.Index), sr); err != nil {
		_ = pw.CloseWithError(err) // 解阻塞写侧，等 goroutine 退出后再返回
		<-done
		return info, err
	}
	// 等写侧完成。ctx 兜底：不消费 pipe 的非法 handler 也不会卡死主流程。
	select {
	case <-done:
	case <-ctx.Done():
		_ = pw.CloseWithError(ctx.Err())
		<-done
		return info, ctx.Err()
	}
	if copyErr != nil {
		return info, copyErr
	}
	info.Size = sr.n

	// 快照成功落远端后再写标记文件。
	if err := r.writeSnapshotMarker(info); err != nil {
		return info, err
	}

	logger.Logger.Info().
		Str("position", pos.String()).
		Dur("elapsed", time.Since(startTime)).
		Int64("bytes", info.Size).
		Msg("snapshot written")
	return info, nil
}

/*
sizeReader 统计被读取的字节数。
*/
type sizeReader struct {
	r io.Reader
	n int64
}

func (s *sizeReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	s.n += int64(n)
	return n, err
}

/*
snapshotMarkerPath 返回 generation 的快照标记文件路径。
*/
func (r *replica) snapshotMarkerPath(generation string) string {
	return filepath.Join(r.generationDir(generation), "snapshot")
}

/*
hasSnapshotMarker 判断 generation 是否已有快照标记。stat 出错视为没有
（走补快照分支，幂等，最多远端多一个文件）。
*/
func (r *replica) hasSnapshotMarker(generation string) bool {
	_, err := os.Stat(r.snapshotMarkerPath(generation))
	return err == nil
}

/*
writeSnapshotMarker 写快照标记文件（内容为快照 index）。
*/
func (r *replica) writeSnapshotMarker(info snapshotInfo) error {
	path := r.snapshotMarkerPath(info.Generation)

	mode := os.FileMode(0600)
	if r.fileInfo != nil {
		mode = r.fileInfo.Mode()
	}
	if err := os.WriteFile(path+".tmp", []byte(fmt.Sprintf("%08x\n", info.Index)), mode); err != nil {
		return err
	}
	uid, gid := fileinfo(r.fileInfo)
	_ = os.Chown(path+".tmp", uid, gid)
	return os.Rename(path+".tmp", path)
}

/*
posPath 返回已上传位置的持久化文件路径。
*/
func (r *replica) posPath() string {
	return filepath.Join(r.metaPath, "replica-pos")
}

/*
loadPosOnce 首次使用时从磁盘加载已上传位置。
*/
func (r *replica) loadPosOnce() {
	if r.posLoaded {
		return
	}
	r.posLoaded = true

	buf, err := os.ReadFile(r.posPath())
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Logger.Warn().Err(err).Msg("loadPos: cannot read pos file, resyncing generation")
		}
		return
	}

	pos := walPos{}
	if err := sonic.Unmarshal(buf, &pos); err != nil {
		logger.Logger.Warn().Err(err).Msg("loadPos: invalid pos file, resyncing generation")
		return
	}
	if !IsGenerationName(pos.Generation) || pos.Index < 0 || pos.Offset < 0 {
		logger.Logger.Warn().Str("position", pos.String()).Msg("loadPos: invalid pos value, resyncing generation")
		return
	}
	r.pos = pos
}

/*
removeRemoteGeneration 删除远端 generation（best-effort，失败仅告警）。
*/
func (r *replica) removeRemoteGeneration(ctx context.Context, generation string) {
	if err := r.handler.Remove(ctx, generation); err != nil {
		logger.Logger.Warn().Err(err).Str("generation", generation).Msg("remove old generation failed")
	}
}

/*
savePos 持久化已上传位置（tmp + rename 原子写）。

正确性依据：shadow 文件只追加不重写 → 同名段重传字节恒等 → pos 回退后
重传也是幂等覆盖；cleanWAL 的多保留一个文件规则保证 pos 指向的 shadow
文件始终在本地。
*/
func (r *replica) savePos() error {
	buf, err := sonic.Marshal(r.pos)
	if err != nil {
		return err
	}

	path := r.posPath()
	mode := os.FileMode(0600)
	if r.fileInfo != nil {
		mode = r.fileInfo.Mode()
	}
	if err := os.WriteFile(path+".tmp", buf, mode); err != nil {
		return err
	}
	uid, gid := fileinfo(r.fileInfo)
	_ = os.Chown(path+".tmp", uid, gid)
	return os.Rename(path+".tmp", path)
}
