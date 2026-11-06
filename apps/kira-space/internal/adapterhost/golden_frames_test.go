package adapterhost

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// goldenFrame is one encodeResponse output the TS decoder spec (tests/unit/frame-golden.spec.ts)
// decodes and asserts field by field, so a Go-side encode slip (a 0 written for an absent optional
// scalar, a dropped null bit) cannot hide behind a test encoder that agrees with the decoder only.
type goldenFrame struct {
	name    string
	payload any
}

func ip(v int) *int      { return &v }
func i64(v int64) *int64 { return &v }

func goldenFrames(t *testing.T) []goldenFrame {
	t.Helper()
	const fetchedAt = 1_700_000_000_000

	tab := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "id", DataType: "bigint", TypeClass: page.TypeClassNumber, IsPrimaryKey: true},
		{Name: "note", DataType: "text", TypeClass: page.TypeClassText, Nullable: true},
		{Name: "doc", DataType: "jsonb", TypeClass: page.TypeClassJSON, Nullable: true, Generated: true},
	})
	long := strings.Repeat("x", page.MaxCellBytes+10)
	for _, row := range [][]*string{
		{strp("1"), strp("hello"), strp(`{"a":1}`)},
		{strp("2"), nil, nil},
		{strp("3"), strp(""), strp(long)},
	} {
		if err := tab.AppendRow(row); err != nil {
			t.Fatal(err)
		}
	}
	keyset := tab.Finish(page.PagePosition{PageSize: 100, HasMore: true, NextToken: strp("next"), PrevToken: strp("prev"), Strategy: "keyset"})
	keyset.FetchedAt = fetchedAt

	offsetTab := page.NewTabularPageBuilder([]page.ColumnDescriptor{{Name: "n", DataType: "int", TypeClass: page.TypeClassNumber}})
	_ = offsetTab.AppendRow([]*string{strp("7")})
	offsetPage := offsetTab.Finish(page.UnpagedPosition(1))
	offsetPage.FetchedAt = fetchedAt

	doc := page.NewDocumentPageBuilder(false)
	doc.Push(`"a"`, `{"k":1}`)
	doc.Push(`"b"`, `{}`)
	docPage := doc.Finish(page.PagePosition{Offset: ip(20), PageSize: 10, HasMore: true, Strategy: "cursor", NextToken: strp("cur")})
	docPage.FetchedAt = fetchedAt

	hash := page.NewKeyValuePageBuilder("hash", i64(5000), i64(88), false)
	hash.Push("f1", "v1")
	hashPage := hash.Finish(page.UnpagedPosition(1))
	hashPage.FetchedAt = fetchedAt

	str := page.NewKeyValuePageBuilder("string", nil, nil, true)
	str.Push("value", "x")
	strPage := str.Finish(page.UnpagedPosition(1))
	strPage.FetchedAt = fetchedAt

	zeroTTL := page.NewKeyValuePageBuilder("object", i64(0), i64(0), true)
	zeroTTL.Push("etag", "abc")
	zeroTTLPage := zeroTTL.Finish(page.UnpagedPosition(1))
	zeroTTLPage.FetchedAt = fetchedAt

	stream := page.NewStreamPageBuilder(ip(30))
	stream.Push(page.StreamRow{Key: strp("k0"), Headers: "{}", Attrs: `{"partition":0}`, Timestamp: strp("2024-01-01T00:00:00.000Z"), Body: strp("body")})
	stream.Push(page.StreamRow{Headers: "{}", Attrs: "{}"})
	streamPage := stream.Finish(page.PagePosition{PageSize: 2, Strategy: "batch"})
	streamPage.FetchedAt = fetchedAt

	noVisibility := page.NewStreamPageBuilder(nil)
	noVisibility.Push(page.StreamRow{Headers: "{}", Attrs: "{}", Body: strp("")})
	noVisibilityPage := noVisibility.Finish(page.PagePosition{Offset: ip(0), PageSize: 1, Strategy: "offsetWindow"})
	noVisibilityPage.FetchedAt = fetchedAt

	return []goldenFrame{
		{"tabular-keyset", ReadResponse{Page: keyset, Source: "server"}},
		{"tabular-offset-cache", ReadResponse{Page: offsetPage, Source: "cache"}},
		{"document-cursor", ReadResponse{Page: docPage, Source: "server"}},
		{"keyvalue-hash-ttl", ReadResponse{Page: hashPage, Source: "server"}},
		{"keyvalue-string-nottl", ReadResponse{Page: strPage, Source: "server"}},
		{"keyvalue-object-zero", ReadResponse{Page: zeroTTLPage, Source: "server"}},
		{"stream-visibility-null-row", ReadResponse{Page: streamPage, Source: "server"}},
		{"stream-novisibility", ReadResponse{Page: noVisibilityPage, Source: "server"}},
	}
}

// TestGoldenFrames_MatchCommittedBytes encodes each payload through encodeResponse (id 7) and
// compares with testdata/frames/<name>.bin. KIRA_IPC_FIXTURES=write rewrites them.
func TestGoldenFrames_MatchCommittedBytes(t *testing.T) {
	for _, g := range goldenFrames(t) {
		t.Run(g.name, func(t *testing.T) {
			got, err := encodeResponse(7, g.payload)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "frames", g.name+".bin")
			if os.Getenv("KIRA_IPC_FIXTURES") == "write" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v (regenerate with KIRA_IPC_FIXTURES=write)", path, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%s: encoded frame differs from golden (%d vs %d bytes); regenerate with KIRA_IPC_FIXTURES=write and update tests/unit/frame-golden.spec.ts", g.name, len(got), len(want))
			}
		})
	}
}
