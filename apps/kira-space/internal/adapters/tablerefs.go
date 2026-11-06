package adapters

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
)

// FuncRef is a routine call the catalog decides (built-in or user).
type FuncRef struct{ Schema, Name string }

// TableParse is what a statement names before any catalog lookup.
type TableParse struct {
	Refs        []tableref.Ref // qualified by the text or the path
	Unqualified []string       // postgres only: relation names search_path resolves
	Funcs       []FuncRef      // calls whose origin the catalog decides
	Ops         []string       // postgres only: unqualified operator names whose schema the catalog decides
	Refusal     string         // non-empty: refused under a list (unparsable, CALL, SET, denied built-in, ...)
}

// NeedsProbe reports whether ResolveTables must run.
func (p TableParse) NeedsProbe() bool {
	return len(p.Unqualified) > 0 || len(p.Funcs) > 0 || len(p.Ops) > 0
}

// TableReferencer names the tables a statement touches (P532). Kinds with tables implement it; redis, kafka,
// s3 and sqs do not.
type TableReferencer interface {
	// ParseTables is pure: no server work.
	ParseTables(statement string, path model.NodePath) TableParse
	// ResolveTables runs the catalog probe for p, called only when p.NeedsProbe(). A non-empty refusal refuses
	// (a user function); err fails the call closed.
	ResolveTables(ctx context.Context, p TableParse, path model.NodePath, op *OpCtx) (refs []tableref.Ref, refusal string, err error)
}
