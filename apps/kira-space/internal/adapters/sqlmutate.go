package adapters

import (
	"context"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// This file is the Go analogue of sql-mutate.ts: the mutation-plan ordering rule and the
// dialect-agnostic renderer four adapters share for UPDATE/DELETE/INSERT text.

var kindRank = map[string]int{"delete": 0, "update": 1, "insert": 2}

// OrderedOps ports sql-mutate.ts's orderedOps: delete, then update, then insert, regardless of the
// plan's own array order — a P5 semantic rule, not a dialect one. sort.SliceStable, not sort.Slice:
// Array.prototype.sort is stable and two ops of the same kind must keep their plan order.
func OrderedOps(ops []model.MutationRowOp) []model.MutationRowOp {
	out := append([]model.MutationRowOp{}, ops...)
	sort.SliceStable(out, func(i, j int) bool {
		return kindRank[out[i].Kind] < kindRank[out[j].Kind]
	})
	return out
}

// AssertColumnsKnown ports sql-mutate.ts's assertColumnsKnown. A generated column is deliberately
// NOT blocked here (P36 D28) — an explicit mutate() call that targets one is left for the server
// to refuse in its own words.
func AssertColumnsKnown(columns []model.ColumnMeta, names []string) error {
	known := make(map[string]struct{}, len(columns))
	for _, c := range columns {
		known[c.Name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := known[name]; !ok {
			return New(CodeNotFound, "unknown column in mutation: "+name, nil)
		}
	}
	return nil
}

// AssertAffectedExactlyOne ports sql-mutate.ts's assertAffectedExactlyOne.
func AssertAffectedExactlyOne(kind string, n int64) error {
	if n != 1 {
		return New(CodeQuery, "expected "+kind+" to affect exactly one row, affected "+strconv.FormatInt(n, 10), nil)
	}
	return nil
}

// AssertKeyIsPrimaryKey ports sql-mutate.ts's assertKeyIsPrimaryKey — a partial or missing primary
// key is not a safe row identifier (P5 D1/D2). qualifiedName is the already-built display string
// each adapter spells its own way.
func AssertKeyIsPrimaryKey(primaryKey []string, key model.RowValues, qualifiedName string) error {
	if len(primaryKey) == 0 {
		return New(CodeUnsupported, qualifiedName+" has no primary key", nil)
	}
	given := append([]string{}, key.Names()...)
	sort.Strings(given)
	pk := append([]string{}, primaryKey...)
	sort.Strings(pk)
	if len(given) != len(pk) {
		return Contract("row key must be exactly the primary key columns", nil)
	}
	for i := range given {
		if given[i] != pk[i] {
			return Contract("row key must be exactly the primary key columns", nil)
		}
	}
	return nil
}

// ValueRenderer is the Go analogue of sql-mutate.ts's ValueRenderer<P> — the one thing the three
// SQL dialects disagree on is the placeholder each renderer emits ($n vs ?), which
// BuildKeysetPredicate already takes as a parameter for the same reason. It also takes the column
// name (NewParamRenderer's own binary-decoding gate below needs it) and can fail: a malformed
// edited value must be reported, not silently mis-encoded.
type ValueRenderer func(name string, value *string, params *[]any) (string, error)

// LiteralRenderer is preview()'s renderer (never executes): an escaped SQL literal, no params
// touched, no decoding — preview text is cosmetic only and never reaches a driver.
func LiteralRenderer(_ string, value *string) (string, error) {
	if value == nil {
		return "NULL", nil
	}
	return "'" + strings.ReplaceAll(*value, "'", "''") + "'", nil
}

// DecodeBinaryCellText decodes the app-wide "0x<hex>" binary-cell display convention (each SQL
// adapter's own read.go cellText/normalizeCellText) back into raw bytes.
func DecodeBinaryCellText(text string) ([]byte, error) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(text, "0x"), "0X")
	decoded, err := hex.DecodeString(trimmed)
	if err != nil {
		return nil, T(CodeQuery, "backend.studio.adapter.binaryValueInvalid.body", nil, err)
	}
	return decoded, nil
}

// NewParamRenderer is mutate()'s renderer — pushes onto params and returns the dialect's
// placeholder for the position it landed at. isBinary reports whether name is a binary-typed
// column: such a column's value arrives here still spelled in the "0x<hex>" display convention
// (read unchanged, or hand-edited) and must be decoded into raw bytes before being bound — handing
// the driver that display string as-is silently corrupts the column's real bytes on every edit,
// since it is the ASCII text "0x4142" that gets stored, not the two bytes it denotes.
func NewParamRenderer(placeholder func(int) string, isBinary func(name string) bool) ValueRenderer {
	return func(name string, value *string, params *[]any) (string, error) {
		if value != nil && isBinary(name) {
			decoded, err := DecodeBinaryCellText(*value)
			if err != nil {
				return "", New(CodeQuery, "column "+name+": "+err.Error(), nil)
			}
			*params = append(*params, decoded)
			return placeholder(len(*params)), nil
		}
		*params = append(*params, value)
		return placeholder(len(*params)), nil
	}
}

func whereFromKey(key model.RowValues, render ValueRenderer, params *[]any, quote func(string) string) (string, error) {
	parts := make([]string, len(key))
	for i, kv := range key {
		if kv.Value == nil {
			parts[i] = quote(kv.Name) + " IS NULL"
		} else {
			rendered, err := render(kv.Name, kv.Value, params)
			if err != nil {
				return "", err
			}
			parts[i] = quote(kv.Name) + " = " + rendered
		}
	}
	return strings.Join(parts, " AND "), nil
}

// RenderRowOp ports sql-mutate.ts's renderRowOp: UPDATE/DELETE/INSERT text for one row op, with
// the WHERE built from the row key. quote is the caller's own quoteIdent.
func RenderRowOp(relationSQL string, op model.MutationRowOp, render ValueRenderer, params *[]any, quote func(string) string) (string, error) {
	switch op.Kind {
	case "update":
		setParts := make([]string, len(op.Changes))
		for i, kv := range op.Changes {
			rendered, err := render(kv.Name, kv.Value, params)
			if err != nil {
				return "", err
			}
			setParts[i] = quote(kv.Name) + " = " + rendered
		}
		where, err := whereFromKey(op.Key, render, params, quote)
		if err != nil {
			return "", err
		}
		return "UPDATE " + relationSQL + " SET " + strings.Join(setParts, ", ") + " WHERE " + where, nil
	case "delete":
		where, err := whereFromKey(op.Key, render, params, quote)
		if err != nil {
			return "", err
		}
		return "DELETE FROM " + relationSQL + " WHERE " + where, nil
	default: // "insert"
		columns := make([]string, len(op.Values))
		values := make([]string, len(op.Values))
		for i, kv := range op.Values {
			columns[i] = quote(kv.Name)
			rendered, err := render(kv.Name, kv.Value, params)
			if err != nil {
				return "", err
			}
			values[i] = rendered
		}
		return "INSERT INTO " + relationSQL + " (" + strings.Join(columns, ", ") + ") VALUES (" +
			strings.Join(values, ", ") + ")", nil
	}
}

// ValidateMutationOps ports the per-op validation loop postgres/mysqlfamily/sqlite's mutate() each
// wrote out verbatim: an update or delete's key must name exactly the primary key, and every
// touched column must be known to the catalog. qualifiedName is the already-built display string
// each adapter spells its own way.
func ValidateMutationOps(ops []model.MutationRowOp, columns []model.ColumnMeta, primaryKey []string, qualifiedName string) error {
	for _, rowOp := range ops {
		switch rowOp.Kind {
		case "update":
			if err := AssertColumnsKnown(columns, append(rowOp.Key.Names(), rowOp.Changes.Names()...)); err != nil {
				return err
			}
			if err := AssertKeyIsPrimaryKey(primaryKey, rowOp.Key, qualifiedName); err != nil {
				return err
			}
		case "delete":
			if err := AssertColumnsKnown(columns, rowOp.Key.Names()); err != nil {
				return err
			}
			if err := AssertKeyIsPrimaryKey(primaryKey, rowOp.Key, qualifiedName); err != nil {
				return err
			}
		default: // "insert"
			if err := AssertColumnsKnown(columns, rowOp.Values.Names()); err != nil {
				return err
			}
		}
	}
	return nil
}

// CompiledOp is one ordered mutation op rendered for execution: the SQL text, its bound params
// (already dialect-placeholdered by paramRenderer) and the op's kind (for AssertAffectedExactlyOne).
type CompiledOp struct {
	SQL    string
	Params []any
	Kind   string
}

// CompileMutationOps ports the render loop postgres/mysqlfamily/sqlite's mutate() each wrote out
// verbatim: one executable statement (via paramRenderer) and one literal preview statement (via
// literalRenderer) per ordered op.
func CompileMutationOps(relationSQL string, ordered []model.MutationRowOp, paramRenderer, literalRenderer ValueRenderer, quote func(string) string) (compiled []CompiledOp, previewParts []string, err error) {
	compiled = make([]CompiledOp, len(ordered))
	previewParts = make([]string, len(ordered))
	for i, rowOp := range ordered {
		var params []any
		sql, err := RenderRowOp(relationSQL, rowOp, paramRenderer, &params, quote)
		if err != nil {
			return nil, nil, err
		}
		compiled[i] = CompiledOp{SQL: sql, Params: params, Kind: rowOp.Kind}
		var literalParams []any
		previewPart, err := RenderRowOp(relationSQL, rowOp, literalRenderer, &literalParams, quote)
		if err != nil {
			return nil, nil, err
		}
		previewParts[i] = previewPart
	}
	return compiled, previewParts, nil
}

// BinaryColumnsOf builds the isBinary lookup NewParamRenderer needs from a read target's own
// resolved column types — a binary column's edited value is still spelled in the "0x<hex>" display
// convention and must be decoded to raw bytes before it reaches the driver. typeClassFor is
// the caller's own dialect-specific mapping.
func BinaryColumnsOf(columns []model.ColumnMeta, typeClassFor func(dataType string) page.TypeClass) func(name string) bool {
	binary := make(map[string]bool, len(columns))
	for _, c := range columns {
		if typeClassFor(c.DataType) == page.TypeClassBinary {
			binary[c.Name] = true
		}
	}
	return func(name string) bool { return binary[name] }
}

// PreviewSQLMutation is mutate.ts's preview — synchronous (D6): no catalog lookup, no network,
// trusts the plan's column names as given.
func PreviewSQLMutation(plan model.MutationPlan, relationSQL string, literalRenderer ValueRenderer, quote func(string) string) ([]string, error) {
	ordered := OrderedOps(plan.Ops)
	statements := make([]string, len(ordered))
	for i, op := range ordered {
		var params []any
		stmt, err := RenderRowOp(relationSQL, op, literalRenderer, &params, quote)
		if err != nil {
			return nil, err
		}
		statements[i] = stmt
	}
	return statements, nil
}

// RunSQLMutation is mutate.ts's own transaction wrapper: begin, exec every compiled op in order
// while asserting each affects exactly one row, commit — with a deferred rollback (never skipped
// on an early return) that only fires when commit never happened. exec is the caller's own
// SuppressCommand-style command executor; rollback runs on its own detached, timeout-bounded ctx so
// it always reaches the server regardless of the caller's own ctx state (common to all three
// adapters' mutate()).
func RunSQLMutation(ctx context.Context, beginSQL string, exec func(sqlText string, params []any) (int64, error), rollback func(context.Context), compiled []CompiledOp) (model.MutationResult, error) {
	if _, err := exec(beginSQL, nil); err != nil {
		return model.MutationResult{}, err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollback(ctx)
	}()
	var affectedRows int64
	for _, c := range compiled {
		n, err := exec(c.SQL, c.Params)
		if err != nil {
			return model.MutationResult{}, err
		}
		if err := AssertAffectedExactlyOne(c.Kind, n); err != nil {
			return model.MutationResult{}, err
		}
		affectedRows += n
	}
	if _, err := exec("COMMIT", nil); err != nil {
		return model.MutationResult{}, err
	}
	committed = true
	return model.MutationResult{AffectedRows: int(affectedRows)}, nil
}

// RelationalMutateDeps is mutate.ts's own mutate body, beyond CompileMutationOps/RunSQLMutation
// above: what postgres/mysqlfamily/sqlite's own mutate() functions still differ on (P107 I2-11).
type RelationalMutateDeps struct {
	// Resolve parses plan.Path, re-fetches the target's live columns/primary key against the
	// catalog (never trusts a stale value the caller sent — same discipline the read path's
	// resolveProjection uses), and reports the fully-qualified relation text mutate's SQL is built
	// against, plus the qualifiedName ValidateMutationOps reports errors against.
	Resolve         func() (relationSQL, qualifiedName string, columns []model.ColumnMeta, primaryKey []string, err error)
	Quote           func(string) string
	Placeholder     func(int) string
	TypeClassFor    func(dataType string) page.TypeClass
	LiteralRenderer ValueRenderer
	BeginSQL        string
	Exec            func(sqlText string, params []any) (int64, error)
	Rollback        func(context.Context)
}

// RunRelationalMutate is mutate.ts's own mutate, shared verbatim across postgres/mysqlfamily/
// sqlite: AssertWritable, resolve+validate, compile, one SetCommand call, then
// RunSQLMutation. deps.Resolve, deps.Exec and deps.Rollback stay adapter-owned because their own
// driver calls (execFor/getReadTarget/runCommand signatures, the rollback statement itself) differ
// per package.
func RunRelationalMutate(ctx context.Context, op *OpCtx, readOnly bool, plan model.MutationPlan, deps RelationalMutateDeps) (model.MutationResult, error) {
	if err := AssertWritable(readOnly); err != nil {
		return model.MutationResult{}, err
	}

	relationSQL, qualifiedName, columns, primaryKey, err := deps.Resolve()
	if err != nil {
		return model.MutationResult{}, err
	}
	if err := ValidateMutationOps(plan.Ops, columns, primaryKey, qualifiedName); err != nil {
		return model.MutationResult{}, err
	}

	paramRenderer := NewParamRenderer(deps.Placeholder, BinaryColumnsOf(columns, deps.TypeClassFor))
	ordered := OrderedOps(plan.Ops)
	compiled, previewParts, err := CompileMutationOps(relationSQL, ordered, paramRenderer, deps.LiteralRenderer, deps.Quote)
	if err != nil {
		return model.MutationResult{}, err
	}
	// One op-log row, one setCommand call, before anything executes (Adapter rule 3, P5 D9).
	op.SetCommand(strings.Join(previewParts, ";\n"))

	return RunSQLMutation(ctx, deps.BeginSQL, deps.Exec, deps.Rollback, compiled)
}

// ResolveDatabaseTablePath ports sql-mutate.ts's resolveDatabaseTablePath — the two-segment
// database/table path check clickhouse/mysql-family/sqlite's mutate.ts each wrote out; postgres
// keeps its own three-segment resolveTablePath.
func ResolveDatabaseTablePath(path model.NodePath) (database, table string, err error) {
	segs, err := RequirePath(path, "mutate", Seg("database"), Seg("table"))
	if err != nil {
		return "", "", err
	}
	return segs[0].Name, segs[1].Name, nil
}
