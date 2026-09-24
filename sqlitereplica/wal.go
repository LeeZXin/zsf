package sqlitereplica

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"
)

// sqliteFileHeader SQLite 库文件头的 16 字节 magic（见 https://www.sqlite.org/fileformat.html）。
var sqliteFileHeader = []byte("SQLite format 3\x00")

/*
hasSQLiteHeader 判断文件头是否为 SQLite 库文件 magic。空文件返回 false 无错。
*/
func hasSQLiteHeader(filename string) (bool, error) {
	f, err := os.Open(filename)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	hdr := make([]byte, len(sqliteFileHeader))
	n, err := io.ReadFull(f, hdr)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return false, nil // 文件太小，没有完整头
	} else if err != nil {
		return false, err
	} else if n != len(hdr) {
		return false, nil
	}
	return bytes.Equal(hdr, sqliteFileHeader), nil
}

// SQLite WAL 二进制格式常量（见 https://www.sqlite.org/fileformat2.html#walformat）。
const (
	// walHeaderSize WAL 文件头大小。
	walHeaderSize = 32
	// walFrameHeaderSize WAL 帧头大小（页号 4B + 提交后库大小 4B + salt 8B + 校验和 8B）。
	walFrameHeaderSize = 24

	// walHeaderChecksumOffset WAL 头内校验和的字节偏移。
	walHeaderChecksumOffset = 24
	// walFrameHeaderChecksumOffset 帧头内校验和的字节偏移。
	walFrameHeaderChecksumOffset = 16

	// maxIndex shadow WAL 最大 index，达到后强制开新 generation。
	maxIndex = 0x7FFFFFFF
)

// SQLite checkpoint 模式（对应 PRAGMA wal_checkpoint 参数）。
const (
	checkpointModePassive  = "PASSIVE"
	checkpointModeRestart  = "RESTART"
	checkpointModeTruncate = "TRUNCATE"
)

// busyTimeout sqlite 连接等待 EBUSY 的超时。
const busyTimeout = 1 * time.Second

/*
checksum 计算 SQLite 滚动校验和（WAL 头与帧校验）。

s0/s1 为上一段的校验和（首段传 0,0），b 长度必须为 8 的倍数。返回新的校验和。
SQLite 官方算法见 wal.c 中的 walChecksumBytes。
*/
func checksum(bo binary.ByteOrder, s0, s1 uint32, b []byte) (uint32, uint32) {
	for i := 0; i < len(b); i += 8 {
		s0 += bo.Uint32(b[i:]) + s1
		s1 += bo.Uint32(b[i+4:]) + s0
	}
	return s0, s1
}

/*
headerByteOrder 根据 WAL 头的 magic 判断字节序。

magic 0x377f0682 表示小端（文件内以大端存储），0x377f0683 表示大端。
*/
func headerByteOrder(hdr []byte) (binary.ByteOrder, error) {
	magic := binary.BigEndian.Uint32(hdr[0:])
	switch magic {
	case 0x377f0682:
		return binary.LittleEndian, nil
	case 0x377f0683:
		return binary.BigEndian, nil
	default:
		return nil, fmt.Errorf("invalid wal header magic: %x", magic)
	}
}

/*
readWALHeader 读取 WAL 文件头（32 字节）。
*/
func readWALHeader(filename string) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, walHeaderSize)
	n, err := io.ReadFull(f, buf)
	return buf[:n], err
}

/*
readWALFileAt 读取文件指定偏移的 n 字节。仅用于 WAL 文件（见 readLastChecksumFrom
与 verify 的尾页比对），不要用于数据库文件，避免非 OFD 锁问题。
*/
func readWALFileAt(filename string, offset, n int64) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, n)
	if n, err := f.ReadAt(buf, offset); err != nil {
		return buf[:n], err
	} else if n < len(buf) {
		return buf[:n], io.ErrUnexpectedEOF
	}
	return buf, nil
}

/*
readLastChecksumFrom 读取 WAL 文件当前末尾的滚动校验和：

  - 仅文件头时返回头校验和
  - 有帧时返回最后一帧的帧校验和

shadow 同步续传时以此为初值，保证跨轮次的校验链不断。
*/
func readLastChecksumFrom(f *os.File, pageSize int) (uint32, uint32, error) {
	offset := int64(walHeaderChecksumOffset)
	if fi, err := f.Stat(); err != nil {
		return 0, 0, err
	} else if sz := frameAlign(fi.Size(), pageSize); fi.Size() > walHeaderSize {
		offset = sz - int64(pageSize) - walFrameHeaderSize + walFrameHeaderChecksumOffset
	}

	b := make([]byte, 8)
	if n, err := f.ReadAt(b, offset); err != nil {
		return 0, 0, err
	} else if n != len(b) {
		return 0, 0, io.ErrUnexpectedEOF
	}
	return binary.BigEndian.Uint32(b[0:]), binary.BigEndian.Uint32(b[4:]), nil
}

/*
frameAlign 返回按帧对齐的 offset（WAL 头 + 完整帧的整数倍）。小于 WAL 头时返回 0。
*/
func frameAlign(offset int64, pageSize int) int64 {
	if offset < walHeaderSize {
		return 0
	}

	frameSize := walFrameHeaderSize + int64(pageSize)
	frameN := (offset - walHeaderSize) / frameSize
	return (frameN * frameSize) + walHeaderSize
}

/*
calcWALSize 返回 n 页帧对应的 WAL 文件总大小。
*/
func calcWALSize(pageSize int, n int) int64 {
	return int64(walHeaderSize + ((walFrameHeaderSize + pageSize) * n))
}
