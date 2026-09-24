package sqlitereplica

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

/*
newTestDB 创建测试库并建表，返回普通 driver 连接（模拟使用方）。
*/
func newTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, v TEXT);`); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertN(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := db.Exec(`INSERT INTO t (v) VALUES (?);`, fmt.Sprintf("value-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
}

func countRows(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM t;`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

// testOptions 测试用 SyncOptions：快速轮询、不周期快照（手动控制）。
func testOptions(dbPath string, h Handler, metaPath string) SyncOptions {
	return SyncOptions{
		Path:             dbPath,
		Handler:          h,
		MetaPath:         metaPath,
		MonitorInterval:  50 * time.Millisecond,
		SnapshotInterval: -1,
	}
}

// posAdvanced 判断 p2 相对 p1 是否推进（Index/Offset 任一增长）。
func posAdvanced(p1, p2 walPos) bool {
	return p2.Index > p1.Index || (p2.Index == p1.Index && p2.Offset > p1.Offset)
}

/*
TestReplicaE2E 端到端：建库写数据 → 同步 → 快照 → 再写 → 关闭 → 损坏原库
（连同 meta 删除模拟灾难）→ 恢复 → 数据一致。
*/
func TestReplicaE2E(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	replicaDir := filepath.Join(dir, "replica")
	metaPath := filepath.Join(dir, "meta")

	db := newTestDB(t, dbPath)
	insertN(t, db, 10)

	r := newReplica(testOptions(dbPath, newTestHandler(replicaDir), metaPath))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = r.Run() }()

	// 等首轮同步（新代快照 + 首段上传）完成。
	waitFor(t, 10*time.Second, func() bool { return !replicaPos(r).IsZero() })

	// 手动快照。
	if _, err := r.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}

	// 先记录 pos 再写数据：50ms 的 monitor 可能抢在写入之后、取 pos 之前
	// 完成同步，若在那之后才取 lastPos，waitFor 会永远等不到「再推进」。
	lastPos := replicaPos(r)
	insertN(t, db, 10)
	waitFor(t, 10*time.Second, func() bool { return posAdvanced(lastPos, replicaPos(r)) })

	// 关闭（内部做最终同步）。
	r.Close()

	// 删除原库与 meta，模拟灾难。
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		_ = os.Remove(p)
	}
	if err := os.RemoveAll(metaPath); err != nil {
		t.Fatal(err)
	}

	// 恢复并校验（灾备场景：原库已删，用包级 Restore）。
	restoredPath := filepath.Join(dir, "restored.db")
	if err := Restore(ctx, newTestHandler(replicaDir), RestoreOptions{OutputPath: restoredPath}); err != nil {
		t.Fatal(err)
	}

	if got := countRows(t, restoredPath); got != 20 {
		t.Fatalf("restored rows = %d, want 20", got)
	}
}

/*
TestReplicaGenerationChange 运行中重建库（WAL 头变化）→ 自动开新 generation，
远端旧代被清理，恢复得到新库数据。
*/
func TestReplicaGenerationChange(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	replicaDir := filepath.Join(dir, "replica")
	metaPath := filepath.Join(dir, "meta")

	db := newTestDB(t, dbPath)
	insertN(t, db, 5)

	h := newTestHandler(replicaDir)
	r1 := newReplica(testOptions(dbPath, h, metaPath))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = r1.Run() }()

	waitFor(t, 10*time.Second, func() bool { return !replicaPos(r1).IsZero() })
	r1.Close()
	gen1 := replicaPos(r1).Generation
	if gen1 != "0000000000000001" {
		t.Fatalf("first generation = %q, want 0000000000000001", gen1)
	}

	// 重建库：WAL 头变化，下次启动必须开新代。
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		_ = os.Remove(p)
	}
	db = newTestDB(t, dbPath)
	insertN(t, db, 3)

	r2 := newReplica(testOptions(dbPath, h, metaPath))
	go func() { _ = r2.Run() }()

	waitFor(t, 10*time.Second, func() bool {
		g := replicaPos(r2).Generation
		return g != "" && g != gen1
	})
	r2.Close()
	if gen2 := replicaPos(r2).Generation; gen2 != "0000000000000002" {
		t.Fatalf("second generation = %q, want 0000000000000002", gen2)
	}

	// 远端只保留最新一代。
	entries, err := os.ReadDir(replicaDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "0000000000000002" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("remote generations = %v, want only [0000000000000002]", names)
	}

	// 恢复 = 新库数据。
	restoredPath := filepath.Join(dir, "restored.db")
	if err := Restore(ctx, h, RestoreOptions{OutputPath: restoredPath}); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, restoredPath); got != 3 {
		t.Fatalf("restored rows = %d, want 3", got)
	}
}

/*
TestReplicaRestartResume 进程重启（新实例 + 同一 meta）：pos 从磁盘恢复，
generation 不变，续传而不是整代重传。
*/
func TestReplicaRestartResume(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	replicaDir := filepath.Join(dir, "replica")
	metaPath := filepath.Join(dir, "meta")

	db := newTestDB(t, dbPath)
	insertN(t, db, 5)

	h := newTestHandler(replicaDir)
	r1 := newReplica(testOptions(dbPath, h, metaPath))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = r1.Run() }()

	waitFor(t, 10*time.Second, func() bool { return !replicaPos(r1).IsZero() })
	r1.Close()
	pos1 := replicaPos(r1)

	// 重启（同一 meta，pos 文件在）。
	r2 := newReplica(testOptions(dbPath, h, metaPath))
	go func() { _ = r2.Run() }()

	waitFor(t, 10*time.Second, func() bool { return !replicaPos(r2).IsZero() })
	pos2 := replicaPos(r2)
	if pos2.Generation != pos1.Generation {
		t.Fatalf("generation changed on restart: %s != %s", pos2.Generation, pos1.Generation)
	}
	if pos2.Index < pos1.Index {
		t.Fatalf("pos went backwards on restart: %s != %s", pos2.String(), pos1.String())
	}

	// 续写并同步后恢复校验。
	insertN(t, db, 5)
	waitFor(t, 10*time.Second, func() bool { return posAdvanced(pos2, replicaPos(r2)) })
	r2.Close()

	restoredPath := filepath.Join(dir, "restored.db")
	if err := Restore(ctx, h, RestoreOptions{OutputPath: restoredPath}); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, restoredPath); got != 10 {
		t.Fatalf("restored rows = %d, want 10", got)
	}
}

/*
TestReplicaCheckpointRestart 大量写入触发 restart checkpoint（WAL 重置），
shadow 开出多个 index 文件，恢复数据完整。
*/
func TestReplicaCheckpointRestart(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	replicaDir := filepath.Join(dir, "replica")
	metaPath := filepath.Join(dir, "meta")

	db := newTestDB(t, dbPath)

	opts := testOptions(dbPath, newTestHandler(replicaDir), metaPath)
	opts.MinCheckpointPageN = 10
	opts.MaxCheckpointPageN = 50
	opts.TruncatePageN = 100000
	r := newReplica(opts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = r.Run() }()

	waitFor(t, 10*time.Second, func() bool { return !replicaPos(r).IsZero() })

	// 每行约 1KB，400 行 ≈ 100+ 页 WAL 帧，超过 MaxCheckpointPageN 触发 RESTART。
	const n = 400
	bigValue := string(make([]byte, 1000))
	for i := 0; i < n; i++ {
		if _, err := db.Exec(`INSERT INTO t (v) VALUES (?);`, fmt.Sprintf("%d-%s", i, bigValue)); err != nil {
			t.Fatal(err)
		}
	}

	// 等 checkpoint 重置 WAL 后 shadow index > 0 且同步追上。
	waitFor(t, 30*time.Second, func() bool { return replicaPos(r).Index > 0 })
	r.Close()

	restoredPath := filepath.Join(dir, "restored.db")
	restorer := newReplica(SyncOptions{Path: dbPath, Handler: newTestHandler(replicaDir)})
	if err := restorer.Restore(ctx, RestoreOptions{OutputPath: restoredPath}); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, restoredPath); got != n {
		t.Fatalf("restored rows = %d, want %d", got, n)
	}
}

/*
flakyHandler 包装 handler，可注入一次性 Output 失败。
*/
type flakyHandler struct {
	Handler
	mu       sync.Mutex
	failNext bool
}

func (h *flakyHandler) Output(ctx context.Context, name string, r io.Reader) error {
	h.mu.Lock()
	fail := h.failNext
	if fail {
		h.failNext = false
	}
	h.mu.Unlock()

	if fail {
		return errors.New("injected output failure")
	}
	return h.Handler.Output(ctx, name, r)
}

/*
TestReplicaOutputFailureRetry 段上传失败后 pos 不推进，下轮同名重试追上。
*/
func TestReplicaOutputFailureRetry(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	replicaDir := filepath.Join(dir, "replica")
	metaPath := filepath.Join(dir, "meta")

	db := newTestDB(t, dbPath)
	insertN(t, db, 5)

	h := &flakyHandler{Handler: newTestHandler(replicaDir)}
	r := newReplica(testOptions(dbPath, h, metaPath))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = r.Run() }()

	waitFor(t, 10*time.Second, func() bool { return !replicaPos(r).IsZero() })
	pos1 := replicaPos(r)

	// 注入一次失败后再写入，等重试追上。
	h.mu.Lock()
	h.failNext = true
	h.mu.Unlock()
	insertN(t, db, 5)

	waitFor(t, 15*time.Second, func() bool { return posAdvanced(pos1, replicaPos(r)) })
	r.Close()

	// 恢复数据完整（重试覆盖了失败的那一段）。
	restoredPath := filepath.Join(dir, "restored.db")
	restorer := newReplica(SyncOptions{Path: dbPath, Handler: newTestHandler(replicaDir)})
	if err := restorer.Restore(ctx, RestoreOptions{OutputPath: restoredPath}); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, restoredPath); got != 10 {
		t.Fatalf("restored rows = %d, want 10", got)
	}
}
