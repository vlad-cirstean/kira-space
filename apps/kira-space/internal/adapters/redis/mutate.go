package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// The reserved sentinels for redis mutations, expressed through the existing relational-shaped
// MutationRowOp rather than widening the shared mutation schema — mirrors mongo/mutate.go's
// $document precedent. keySentinel names the target redis key: plan.path only ever resolves to a
// database, never a specific key, so every op carries its own key name — including insert, which
// by definition has no existing key a path could point at yet.
const keySentinel = "_key"

// resolveDatabaseSegment is mutate's own database-rooted path check — the renderer may send a key's
// own path below the database (a namespaced-key delete), so only the leading database segment is
// checked, at any depth.
func resolveDatabaseSegment(path model.NodePath) (string, error) {
	segs, err := adapters.RequirePathPrefix(path, "mutate", adapters.Seg("database"))
	if err != nil {
		return "", err
	}
	return segs[0].Name, nil
}

func keyNameFrom(values model.RowValues, label string) (string, error) {
	raw, ok := values.Get(keySentinel)
	if !ok || raw == nil || *raw == "" {
		return "", adapters.New(adapters.CodeQuery, "a redis "+label+" mutation requires a non-empty "+keySentinel, nil)
	}
	return *raw, nil
}

func renderOpText(op model.MutationRowOp) (string, error) {
	switch op.Kind {
	case "update":
		key, err := keyNameFrom(op.Key, "update")
		if err != nil {
			return "", err
		}
		value, err := adapters.ValueFrom(op.Changes, "update")
		if err != nil {
			return "", err
		}
		// KEEPTTL: editing a value must not silently clear whatever expiry the key already had —
		// a plain SET (this adapter's own prior behaviour) resets TTL to none, which is a real
		// data-loss surprise on every edit of a key that happens to expire (P2 R1 finding).
		return "SET " + key + " " + value + " KEEPTTL", nil
	case "delete":
		key, err := keyNameFrom(op.Key, "delete")
		if err != nil {
			return "", err
		}
		return "DEL " + key, nil
	default: // insert
		key, err := keyNameFrom(op.Values, "insert")
		if err != nil {
			return "", err
		}
		value, err := adapters.ValueFrom(op.Values, "insert")
		if err != nil {
			return "", err
		}
		return "SET " + key + " " + value + " NX", nil
	}
}

// preview is mutate.ts's preview — synchronous (Adapter rule 3): no network, no TYPE lookup.
func preview(plan model.MutationPlan) ([]string, error) {
	if _, err := resolveDatabaseSegment(plan.Path); err != nil {
		return nil, err
	}
	out := make([]string, len(plan.Ops))
	for i, op := range plan.Ops {
		text, err := renderOpText(op)
		if err != nil {
			return nil, err
		}
		out[i] = text
	}
	return out, nil
}

// assertEditableType ports mutate.ts's assertEditableType: edit is scoped to string-type keys
// only (documented scope decision) — a hash/list/set/zset/stream each has its own per-element
// mutation semantics, a materially bigger job than a single SET.
func assertEditableType(ctx context.Context, conn *goredis.Client, key string) error {
	rawType, err := conn.Type(ctx, key).Result()
	if err != nil {
		return mapError(err)
	}
	if rawType != "none" && rawType != "string" {
		return adapters.New(adapters.CodeUnsupported, "only string-type keys are editable in this version, "+key+" is "+rawType, nil)
	}
	return nil
}

// applyUpdate is mutateDB's "update" arm.
func applyUpdate(ctx context.Context, conn *goredis.Client, rowOp model.MutationRowOp) (int, error) {
	key, err := keyNameFrom(rowOp.Key, "update")
	if err != nil {
		return 0, err
	}
	value, err := adapters.ValueFrom(rowOp.Changes, "update")
	if err != nil {
		return 0, err
	}
	if err := assertEditableType(ctx, conn, key); err != nil {
		return 0, err
	}
	// goredis.KeepTTL, not 0: a plain SET clears the key's existing expiry, silently
	// dropping a TTL the user never asked to change (P2 R1 finding).
	if err := conn.Set(ctx, key, value, goredis.KeepTTL).Err(); err != nil {
		return 0, mapError(err)
	}
	return 1, nil
}

// applyDelete is mutateDB's "delete" arm. DEL is type-agnostic — works for any of the six redis
// types alike.
func applyDelete(ctx context.Context, conn *goredis.Client, rowOp model.MutationRowOp) (int, error) {
	key, err := keyNameFrom(rowOp.Key, "delete")
	if err != nil {
		return 0, err
	}
	deleted, err := conn.Del(ctx, key).Result()
	if err != nil {
		return 0, mapError(err)
	}
	return int(deleted), nil
}

// applyInsert is mutateDB's default (insert) arm: NX — creating a brand-new key must never
// silently overwrite an existing one; that's what update (a plain SET) is for. A collision
// surfaces as a query-time condition, not a connection failure.
func applyInsert(ctx context.Context, conn *goredis.Client, rowOp model.MutationRowOp) (int, error) {
	key, err := keyNameFrom(rowOp.Values, "insert")
	if err != nil {
		return 0, err
	}
	value, err := adapters.ValueFrom(rowOp.Values, "insert")
	if err != nil {
		return 0, err
	}
	created, err := conn.SetNX(ctx, key, value, 0).Result()
	if err != nil {
		return 0, mapError(err)
	}
	if !created {
		return 0, adapters.New(adapters.CodeQuery, "key already exists: "+key, nil)
	}
	return 1, nil
}

// mutateDB is mutate.ts's mutate.
func mutateDB(ctx context.Context, conn *goredis.Client, op *adapters.OpCtx, readOnly bool, plan model.MutationPlan) (model.MutationResult, error) {
	// §8.12's standard: enforced here, not only greyed out in the UI.
	if err := adapters.AssertWritable(readOnly); err != nil {
		return model.MutationResult{}, err
	}
	if _, err := resolveDatabaseSegment(plan.Path); err != nil {
		return model.MutationResult{}, err
	}

	statements, err := preview(plan)
	if err != nil {
		return model.MutationResult{}, err
	}

	return adapters.RunKindDispatched(ctx, op, plan, readOnly, statements, adapters.DispatchUpdateDeleteInsert(
		func(ctx context.Context, rowOp model.MutationRowOp) (int, error) {
			return applyUpdate(ctx, conn, rowOp)
		},
		func(ctx context.Context, rowOp model.MutationRowOp) (int, error) {
			return applyDelete(ctx, conn, rowOp)
		},
		func(ctx context.Context, rowOp model.MutationRowOp) (int, error) {
			return applyInsert(ctx, conn, rowOp)
		},
	))
}
