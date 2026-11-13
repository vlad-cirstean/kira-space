package repos

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// The trim picks the shortest oldest prefix reaching the excess; compare against dropping chunks
// one by one.
func TestAdeLogsAppendTrim(t *testing.T) {
	const cap = AdeLogTailBytes
	full := strings.Repeat("a", AdeLogChunkBytes)
	cases := []struct {
		name  string
		sizes func() []string
	}{
		{"nothing to trim", func() []string { return []string{"x", "yy"} }},
		{"cap hit exactly", func() []string {
			out := repeatChunks(full, cap/AdeLogChunkBytes)
			return out
		}},
		{"cap exceeded by one byte", func() []string { return append(repeatChunks(full, cap/AdeLogChunkBytes), "z") }},
		{"multi-byte text", func() []string {
			return append(repeatChunks(strings.Repeat("é€", AdeLogChunkBytes/5), cap/AdeLogChunkBytes), "日本語")
		}},
		{"prefix wider than the first scan", func() []string {
			return append(repeatChunks("c", 300), repeatChunks(full, cap/AdeLogChunkBytes+1)...)
		}},
		{"one chunk covers the excess", func() []string { return append([]string{full, "tiny"}, repeatChunks("b", 10)...) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sdb, err := storage.OpenAt(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sdb.Close() })
			all, err := New(sdb.DB)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := all.AdeTasks.CreateTask(model.AdeTask{ID: "t1", Kind: "task", Title: "t1", CreatedAt: 1}, nil, nil, nil); err != nil {
				t.Fatal(err)
			}
			r := all.AdeLogs
			if err := r.Reset(AdeLogRun, "r1", "t1"); err != nil {
				t.Fatal(err)
			}
			texts := c.sizes()
			if c.name == "one chunk covers the excess" {
				// fill to the cap first so the final append overflows by under one chunk
				texts = append(repeatChunks(full, cap/AdeLogChunkBytes-1), texts...)
			}
			var chunks []AdeLogChunk
			for _, s := range texts {
				chunks = append(chunks, AdeLogChunk{At: 1, Stream: "stdout", Text: s})
			}
			for i := 0; i < len(chunks); i += 64 {
				if _, err := r.Append(AdeLogRun, "r1", chunks[i:min(i+64, len(chunks))]); err != nil {
					t.Fatal(err)
				}
			}
			// brute force: drop the oldest one by one while over the cap
			sizes := make([]int, len(texts))
			total := 0
			for i, s := range texts {
				sizes[i] = len(s)
				total += len(s)
			}
			first := 0
			for total > cap {
				total -= sizes[first]
				first++
			}
			rows, err := sdb.DB.Query(`SELECT seq, length(CAST(text AS BLOB)) FROM ade_log_chunks WHERE kind = ? AND id = ? ORDER BY seq`, AdeLogRun, "r1")
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			n, sum := 0, 0
			for rows.Next() {
				var seq, size int
				if err := rows.Scan(&seq, &size); err != nil {
					t.Fatal(err)
				}
				if seq != first+n+1 || size != sizes[first+n] {
					t.Fatalf("chunk %d: seq %d size %d, want seq %d size %d", n, seq, size, first+n+1, sizes[first+n])
				}
				n++
				sum += size
			}
			if n != len(texts)-first {
				t.Fatalf("kept %d chunks, want %d", n, len(texts)-first)
			}
			var bytes int
			var truncated int
			if err := sdb.DB.QueryRow(`SELECT bytes, truncated FROM ade_logs WHERE kind = ? AND id = ?`, AdeLogRun, "r1").Scan(&bytes, &truncated); err != nil {
				t.Fatal(err)
			}
			if bytes != sum || bytes != total || (first > 0) != (truncated == 1) {
				t.Fatalf("parent bytes %d truncated %d, want %d (kept sum %d) truncated %v", bytes, truncated, total, sum, first > 0)
			}
		})
	}
}

func repeatChunks(text string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = text
	}
	return out
}
