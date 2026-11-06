package adapters

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// RunRowOps is redis/mongo/s3/sqs's own mutate loop, carried verbatim across all four: readOnly
// guard, iterate plan.Ops, call apply per op, accumulate the affected count, assemble the result.
// apply owns the per-op-kind switch and any per-engine error mapping; i is the op's own index in
// plan.Ops, for an apply that wants it.
func RunRowOps(ctx context.Context, plan model.MutationPlan, readOnly bool, apply func(ctx context.Context, i int, op model.MutationRowOp) (affected int, err error)) (model.MutationResult, error) {
	if err := AssertWritable(readOnly); err != nil {
		return model.MutationResult{}, err
	}
	affectedRows := 0
	for i, op := range plan.Ops {
		if err := CheckCancelled(ctx); err != nil {
			return model.MutationResult{}, err
		}
		affected, err := apply(ctx, i, op)
		if err != nil {
			return model.MutationResult{}, err
		}
		affectedRows += affected
	}
	return model.MutationResult{AffectedRows: affectedRows}, nil
}

// RunKindDispatched folds mutate's own statements-join + SetCommand + RunRowOps call, repeated
// across mongo/redis/s3/sqs with a hand-rolled ";\n" join loop the strings.Join replacement missed.
// dispatch is the caller's own per-op-kind switch — sqs's own
// switch (no true "update" case, an unsupported error instead) genuinely differs from
// mongo/redis/s3's shared "update/delete/default-insert" shape (DispatchUpdateDeleteInsert
// below), so the switch itself is the caller's, not folded in here.
func RunKindDispatched(ctx context.Context, op *OpCtx, plan model.MutationPlan, readOnly bool, statements []string, dispatch func(ctx context.Context, i int, rowOp model.MutationRowOp) (int, error)) (model.MutationResult, error) {
	op.SetCommand(strings.Join(statements, ";\n"))
	return RunRowOps(ctx, plan, readOnly, dispatch)
}

// DispatchUpdateDeleteInsert wraps mongo/redis/s3's own shared per-op switch (update, delete,
// default insert) as a RunKindDispatched dispatch func (P107 I2-12).
func DispatchUpdateDeleteInsert(update, delete, insert func(ctx context.Context, rowOp model.MutationRowOp) (int, error)) func(ctx context.Context, i int, rowOp model.MutationRowOp) (int, error) {
	return func(ctx context.Context, _ int, rowOp model.MutationRowOp) (int, error) {
		switch rowOp.Kind {
		case "update":
			return update(ctx, rowOp)
		case "delete":
			return delete(ctx, rowOp)
		default: // insert
			return insert(ctx, rowOp)
		}
	}
}

// HeaderPair is one $headers entry, in the order the JSON object spelled it.
type HeaderPair struct{ Key, Value string }

// ParseHeaderPairs decodes a $headers value (a flat JSON object of string values, or absent) in
// document order, keeping repeated names: Kafka headers are an ordered list that may repeat a key.
func ParseHeaderPairs(raw *string) ([]HeaderPair, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	malformed := func(err error) error {
		return T(CodeQuery, "backend.studio.adapter.headersJsonMalformed.body", nil, err)
	}
	dec := json.NewDecoder(strings.NewReader(*raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, malformed(err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, T(CodeQuery, "backend.studio.adapter.headersNotObject.body", nil, nil)
	}
	var out []HeaderPair
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, malformed(err)
		}
		key, _ := keyTok.(string)
		valTok, err := dec.Token()
		if err != nil {
			return nil, malformed(err)
		}
		value, ok := valTok.(string)
		if !ok {
			return nil, New(CodeQuery, "$headers."+key+" must be a string", nil)
		}
		out = append(out, HeaderPair{Key: key, Value: value})
	}
	if _, err := dec.Token(); err != nil {
		return nil, malformed(err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, malformed(err)
	}
	return out, nil
}

// ParseHeaderJSON is sqs/mutate.go's parseHeaders: ParseHeaderPairs as a map (a repeated name
// keeps its last value), since SQS message attributes are unordered and unique.
func ParseHeaderJSON(raw *string) (map[string]string, error) {
	pairs, err := ParseHeaderPairs(raw)
	if err != nil || pairs == nil {
		return nil, err
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		out[p.Key] = p.Value
	}
	return out, nil
}

// ValueFrom is redis/mutate.go's valueFrom == s3/mutate.go's valueFrom: the required, non-nil
// (but possibly empty) $value sentinel.
func ValueFrom(values model.RowValues, label string) (string, error) {
	raw, ok := values.Get("$value")
	if !ok || raw == nil {
		return "", New(CodeUnsupported, "a "+label+" mutation requires a $value", nil)
	}
	return *raw, nil
}

// PreviewProduce is kafka/produce.go's preview == sqs/mutate.go's preview: render each op via the
// caller's own renderOpText, keyed off one target name (topic or queue).
func PreviewProduce(plan model.MutationPlan, target string, renderOpText func(op model.MutationRowOp, target string) (string, error)) ([]string, error) {
	out := make([]string, len(plan.Ops))
	for i, op := range plan.Ops {
		text, err := renderOpText(op, target)
		if err != nil {
			return nil, err
		}
		out[i] = text
	}
	return out, nil
}
