package ipcfixture

import (
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// The four LogicalPage variants, ported field-for-field from tests/ipc/support/types.ts. Each
// reuses page.ColumnDescriptor/page.PagePosition directly rather than redeclaring them — those
// already carry the exact JSON tags and field order the fixtures were captured against.
// truncatedRows (types.ts) is a tests/ui/-only field decodePage() itself never produces; this
// package, like the TypeScript decoder it replaces, does not emit it either.

// LogicalTabularPage is types.ts's LogicalTabularPage.
type LogicalTabularPage struct {
	Kind           string                  `json:"kind"`
	Columns        []page.ColumnDescriptor `json:"columns"`
	Rows           [][]*string             `json:"rows"`
	Position       page.PagePosition       `json:"position"`
	TruncatedCells int                     `json:"truncatedCells"`
}

// LogicalDocumentPage is types.ts's LogicalDocumentPage.
type LogicalDocumentPage struct {
	Kind     string            `json:"kind"`
	IDs      []*string         `json:"ids"`
	Bodies   []*string         `json:"bodies"`
	Position page.PagePosition `json:"position"`
}

// LogicalKeyValuePage is types.ts's LogicalKeyValuePage.
type LogicalKeyValuePage struct {
	Kind        string            `json:"kind"`
	RedisType   string            `json:"redisType"`
	TTLMs       *int64            `json:"ttlMs"`
	MemoryBytes *int64            `json:"memoryBytes"`
	Fields      []*string         `json:"fields"`
	Values      []*string         `json:"values"`
	Position    page.PagePosition `json:"position"`
}

// LogicalStreamPage is types.ts's LogicalStreamPage.
type LogicalStreamPage struct {
	Kind                     string            `json:"kind"`
	Keys                     []*string         `json:"keys"`
	Headers                  []*string         `json:"headers"`
	Attrs                    []*string         `json:"attrs"`
	Timestamps               []*string         `json:"timestamps"`
	Bodies                   []*string         `json:"bodies"`
	Position                 page.PagePosition `json:"position"`
	VisibilityTimeoutSeconds *int              `json:"visibilityTimeoutSeconds"`
}

// decodeColumn is decode.ts's decodeColumn, built on page.IsNull/page.CellText — never a
// hand-rolled reimplementation (P50 D6, restated by P58f §4.3(b) for this port).
func decodeColumn(chunk page.Chunk, rowCount int) []*string {
	out := make([]*string, rowCount)
	for row := 0; row < rowCount; row++ {
		if page.IsNull(chunk, row) {
			continue
		}
		s := page.CellText(chunk, row)
		out[row] = &s
	}
	return out
}

// DecodePage is decode.ts's decodePage: a real page.Page -> one of the four Logical*Page structs
// above, dropping fetchedAt/byteSize (wall-clock/size-derived, would churn every fixture).
func DecodePage(p page.Page) (any, error) {
	switch v := p.(type) {
	case page.TabularPage:
		rows := make([][]*string, v.RowCount)
		for row := 0; row < v.RowCount; row++ {
			r := make([]*string, len(v.Chunks))
			for ci, chunk := range v.Chunks {
				if page.IsNull(chunk, row) {
					continue
				}
				s := page.CellText(chunk, row)
				r[ci] = &s
			}
			rows[row] = r
		}
		return LogicalTabularPage{
			Kind: "tabular", Columns: v.Columns, Rows: rows, Position: v.Position,
			TruncatedCells: v.TruncatedCells,
		}, nil
	case page.DocumentPage:
		return LogicalDocumentPage{
			Kind: "document", IDs: decodeColumn(v.IDs, v.RowCount), Bodies: decodeColumn(v.Bodies, v.RowCount),
			Position: v.Position,
		}, nil
	case page.KeyValuePage:
		return LogicalKeyValuePage{
			Kind: "keyvalue", RedisType: v.RedisType, TTLMs: v.TTLMs, MemoryBytes: v.MemoryBytes,
			Fields: decodeColumn(v.Fields, v.RowCount), Values: decodeColumn(v.Values, v.RowCount),
			Position: v.Position,
		}, nil
	case page.StreamPage:
		return LogicalStreamPage{
			Kind: "stream", Keys: decodeColumn(v.Keys, v.RowCount), Headers: decodeColumn(v.Headers, v.RowCount),
			Attrs: decodeColumn(v.Attrs, v.RowCount), Timestamps: decodeColumn(v.Timestamps, v.RowCount),
			Bodies: decodeColumn(v.Bodies, v.RowCount), Position: v.Position,
			VisibilityTimeoutSeconds: v.VisibilityTimeoutSeconds,
		}, nil
	default:
		return nil, fmt.Errorf("ipcfixture: unknown page kind %T", p)
	}
}
