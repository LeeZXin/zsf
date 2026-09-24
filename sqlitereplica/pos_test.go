package sqlitereplica

import "testing"

func TestSnapshotPathFormatParse(t *testing.T) {
	tests := []struct {
		index int
	}{
		{0}, {1}, {0x7FFFFFFF},
	}
	for _, tt := range tests {
		name := formatSnapshotPath(tt.index)
		idx, err := parseSnapshotPath(name)
		if err != nil {
			t.Fatalf("parseSnapshotPath(%q) error: %v", name, err)
		} else if idx != tt.index {
			t.Fatalf("parseSnapshotPath(%q) = %d, want %d", name, idx, tt.index)
		}
	}

	// 非法输入。
	for _, name := range []string{"", "00000000.snapshot", "0000000.snapshot.lz4", "000000000.snapshot.lz4", "g0000000.snapshot.lz4", "00000000.wal.lz4"} {
		if _, err := parseSnapshotPath(name); err == nil {
			t.Fatalf("parseSnapshotPath(%q) expected error", name)
		}
	}
}

func TestWALSegmentPathFormatParse(t *testing.T) {
	tests := []struct {
		index  int
		offset int64
	}{
		{0, 0}, {1, 4096}, {0x7FFFFFFF, 0x7FFFFFFF},
	}
	for _, tt := range tests {
		name := formatWALSegmentPath(tt.index, tt.offset)
		index, offset, err := parseWALSegmentPath(name)
		if err != nil {
			t.Fatalf("parseWALSegmentPath(%q) error: %v", name, err)
		} else if index != tt.index || offset != tt.offset {
			t.Fatalf("parseWALSegmentPath(%q) = (%d,%d), want (%d,%d)", name, index, offset, tt.index, tt.offset)
		}
	}

	for _, name := range []string{"", "00000000_00000000.wal", "00000000_0000000.wal.lz4", "000000000_00000000.wal.lz4", "00000000_00000000x.wal.lz4"} {
		if _, _, err := parseWALSegmentPath(name); err == nil {
			t.Fatalf("parseWALSegmentPath(%q) expected error", name)
		}
	}
}

func TestIsGenerationName(t *testing.T) {
	for _, s := range []string{"0000000000000001", "ffffffffffffffff", "0123456789abcdef"} {
		if !IsGenerationName(s) {
			t.Fatalf("IsGenerationName(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "000000000000001", "00000000000000001", "000000000000000g", "FFFFFFFFFFFFFFFF", "0000000000000001/"} {
		if IsGenerationName(s) {
			t.Fatalf("IsGenerationName(%q) = true, want false", s)
		}
	}
}

func TestPosString(t *testing.T) {
	if s := (walPos{}).String(); s != "" {
		t.Fatalf("zero walPos.String() = %q, want empty", s)
	}
	p := walPos{Generation: "0000000000000001", Index: 2, Offset: 4096}
	if got, want := p.String(), "0000000000000001/00000002:4096"; got != want {
		t.Fatalf("walPos.String() = %q, want %q", got, want)
	}
}

func TestFrameAlign(t *testing.T) {
	const pageSize = 4096
	const frameSize = walFrameHeaderSize + pageSize

	tests := []struct {
		offset int64
		want   int64
	}{
		{0, 0},
		{walHeaderSize - 1, 0},
		{walHeaderSize, walHeaderSize},
		{walHeaderSize + frameSize - 1, walHeaderSize},
		{walHeaderSize + frameSize, walHeaderSize + frameSize},
		{walHeaderSize + frameSize*3 + 10, walHeaderSize + frameSize*3},
	}
	for _, tt := range tests {
		if got := frameAlign(tt.offset, pageSize); got != tt.want {
			t.Fatalf("frameAlign(%d) = %d, want %d", tt.offset, got, tt.want)
		}
	}
}

func TestCalcWALSize(t *testing.T) {
	const pageSize = 4096
	if got := calcWALSize(pageSize, 0); got != walHeaderSize {
		t.Fatalf("calcWALSize(0) = %d, want %d", got, walHeaderSize)
	}
	if got := calcWALSize(pageSize, 3); got != int64(walHeaderSize+(walFrameHeaderSize+pageSize)*3) {
		t.Fatalf("calcWALSize(3) = %d, want %d", got, walHeaderSize+(walFrameHeaderSize+pageSize)*3)
	}
}

func TestParseShadowWALName(t *testing.T) {
	if idx, err := parseShadowWALName("00000002.wal"); err != nil || idx != 2 {
		t.Fatalf("parseShadowWALName(00000002.wal) = (%d, %v), want (2, nil)", idx, err)
	}
	for _, name := range []string{"", "0000002.wal", "00000002.wal.lz4", "g0000002.wal"} {
		if _, err := parseShadowWALName(name); err == nil {
			t.Fatalf("parseShadowWALName(%q) expected error", name)
		}
	}
}
