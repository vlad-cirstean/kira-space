package mobileterm

import (
	"bytes"
	"testing"
)

func TestRing_WrapAndResume(t *testing.T) {
	r := newRing(8, 0)
	r.Append([]byte("abcdef"))
	r.Append([]byte("ghij")) // window now "cdefghij", start 2
	data, end := r.Snapshot()
	if string(data) != "cdefghij" || end != 10 {
		t.Fatalf("snapshot = %q, %d", data, end)
	}
	data, next, over := r.ReadFrom(6, 100)
	if string(data) != "ghij" || next != 10 || over {
		t.Fatalf("resume = %q, %d, %v", data, next, over)
	}
	data, next, over = r.ReadFrom(1, 3)
	if string(data) != "cde" || next != 5 || !over {
		t.Fatalf("overrun = %q, %d, %v", data, next, over)
	}
	if data, next, _ = r.ReadFrom(10, 4); len(data) != 0 || next != 10 {
		t.Fatalf("at end = %q, %d", data, next)
	}
}

func TestRing_OversizeAppendKeepsTail(t *testing.T) {
	r := newRing(4, 0)
	r.Append([]byte("0123456789"))
	data, end := r.Snapshot()
	if !bytes.Equal(data, []byte("6789")) || end != 10 {
		t.Fatalf("snapshot = %q, %d", data, end)
	}
}

func TestRing_GrowThenWrap(t *testing.T) {
	r := newRing(8, 0)
	r.Append([]byte("abc"))
	if len(r.buf) != 3 {
		t.Fatalf("buf = %d, want lazy growth", len(r.buf))
	}
	r.Append([]byte("defgh")) // exactly full
	r.Append([]byte("ij"))    // wraps
	data, end := r.Snapshot()
	if string(data) != "cdefghij" || end != 10 {
		t.Fatalf("snapshot = %q, %d", data, end)
	}

	r = newRing(8, 0)
	r.Append([]byte("abcde"))
	r.Append([]byte("fghijk")) // straddles the grow-to-wrap boundary
	data, end = r.Snapshot()
	if string(data) != "defghijk" || end != 11 {
		t.Fatalf("straddle = %q, %d", data, end)
	}

	r = newRing(4, 0)
	r.Append([]byte("ab"))
	r.Append([]byte("0123456789")) // oversize onto a partial buffer
	data, end = r.Snapshot()
	if string(data) != "6789" || end != 12 {
		t.Fatalf("oversize = %q, %d", data, end)
	}
}

func TestRing_BaseMakesStaleOffsetOverrun(t *testing.T) {
	r := newRing(8, 50000)
	r.Append([]byte("abc"))
	if r.End() != 50003 {
		t.Fatalf("end = %d", r.End())
	}
	data, next, over := r.ReadFrom(40000, 10)
	if string(data) != "abc" || next != 50003 || !over {
		t.Fatalf("stale = %q, %d, %v", data, next, over)
	}
	data, next, over = r.ReadFrom(50001, 10)
	if string(data) != "bc" || next != 50003 || over {
		t.Fatalf("resume = %q, %d, %v", data, next, over)
	}
}
