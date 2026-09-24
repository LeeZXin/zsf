package sqlitereplica

import (
	"database/sql"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

/*
TestChecksumWALHeaderGolden 用 SQLite 真实生成的 WAL 头做 golden 校验：
读出的头内存储的校验和必须等于 checksum 计算结果。
*/
func TestChecksumWALHeaderGolden(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// 用组件自定义 driver（PERSIST_WAL），连接关闭后 WAL 不被删除。
	db, err := sql.Open(driverName, dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// 开启 WAL 并写一笔，产生真实 WAL 文件。
	if _, err := db.Exec(`PRAGMA journal_mode = wal;`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO t (id) VALUES (1);`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	hdr, err := readWALHeader(dbPath + "-wal")
	if err != nil {
		t.Fatal(err)
	}

	bo, err := headerByteOrder(hdr)
	if err != nil {
		t.Fatal(err)
	}

	s0 := binary.BigEndian.Uint32(hdr[walHeaderChecksumOffset:])
	s1 := binary.BigEndian.Uint32(hdr[walHeaderChecksumOffset+4:])
	v0, v1 := checksum(bo, 0, 0, hdr[:walHeaderChecksumOffset])
	if v0 != s0 || v1 != s1 {
		t.Fatalf("header checksum mismatch: computed (%x,%x) != stored (%x,%x)", v0, v1, s0, s1)
	}
}

/*
TestReadLastChecksumFrom 校验读出的末尾校验和与逐帧滚动计算的最终值一致。
*/
func TestReadLastChecksumFrom(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// 用组件自定义 driver（PERSIST_WAL），连接关闭后 WAL 不被删除。
	db, err := sql.Open(driverName, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode = wal;`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY);`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := db.Exec(`INSERT INTO t (id) VALUES (?);`, i); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	walPath := dbPath + "-wal"
	hdr, err := readWALHeader(walPath)
	if err != nil {
		t.Fatal(err)
	}
	bo, err := headerByteOrder(hdr)
	if err != nil {
		t.Fatal(err)
	}
	const pageSize = 4096

	f, err := os.Open(walPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	// 逐帧滚动计算最终校验和。
	buf, err := os.ReadFile(walPath)
	if err != nil {
		t.Fatal(err)
	}
	body := buf[walHeaderSize:]
	frameSize := walFrameHeaderSize + pageSize
	body = body[:len(body)/frameSize*frameSize]

	s0, s1 := checksum(bo, 0, 0, hdr[:walHeaderChecksumOffset])
	for i := 0; i < len(body); i += frameSize {
		frame := body[i : i+frameSize]
		s0, s1 = checksum(bo, s0, s1, frame[:8])  // 帧头
		s0, s1 = checksum(bo, s0, s1, frame[24:]) // 帧数据
	}

	want0, want1, err := readLastChecksumFrom(f, pageSize)
	if err != nil {
		t.Fatal(err)
	}
	if s0 != want0 || s1 != want1 {
		t.Fatalf("last checksum mismatch: computed (%x,%x) != read (%x,%x)", s0, s1, want0, want1)
	}
}
