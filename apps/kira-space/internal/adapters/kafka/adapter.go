package kafka

import (
	"context"
	"strconv"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func init() {
	adapters.Register("kafka", func(deps adapters.Deps) (adapters.Adapter, error) {
		return &Adapter{deps: deps}, nil
	})
}

// connState is every field Connect/Disconnect write concurrently with an in-flight op reading them
// (F3), guarded together via adapters.Guarded (P113 G1).
type connState struct {
	client *kgo.Client
	admin  *kadm.Client
	// opts is the resolved seed/security options connect() built the long-lived client from —
	// reused to build each browse's own ephemeral kgo.Client without re-resolving the
	// connection config on every read(). Reassigned wholesale by setConnected/clearConnected, never
	// mutated in place, so Load's own copy is safe for it.
	opts     []kgo.Opt
	readOnly bool
}

// Adapter is index.ts's KafkaAdapter.
type Adapter struct {
	deps adapters.Deps

	state adapters.Guarded[connState]
}

// setConnected is Connect's own locked write of every field a successful connect fills in (F3).
func (a *Adapter) setConnected(client *kgo.Client, admin *kadm.Client, opts []kgo.Opt, readOnly bool) {
	a.state.Update(func(s *connState) {
		s.client = client
		s.admin = admin
		s.opts = opts
		s.readOnly = readOnly
	})
}

// clearConnected is Disconnect's own locked write, once client.Close (a real network call, run
// with no lock held) has returned (F3).
func (a *Adapter) clearConnected() {
	a.state.Update(func(s *connState) {
		s.client = nil
		s.admin = nil
		s.opts = nil
	})
}

func (a *Adapter) Kind() string        { return "kafka" }
func (a *Adapter) Caps() adapters.Caps { return caps }

// Connect is index.ts's connect, extended per P58e E15: kgo.NewClient -> Ping -> kadm.Metadata,
// which recovers a real cluster ID (P32 D13, previously lost) alongside a live, cluster-wide
// broker count (previously only the configured bootstrap-address count).
func (a *Adapter) Connect(ctx context.Context, cfg model.ResolvedConnectionConfig, op *adapters.OpCtx) (adapters.ConnectInfo, error) {
	cl, adm, opts, err := connect(ctx, cfg, a.deps.Log)
	if err != nil {
		return adapters.ConnectInfo{}, err
	}
	var meta kadm.Metadata
	err = closeOnError(ctx, cl, func() (err error) {
		meta, err = adm.Metadata(ctx)
		return err
	})
	if err != nil {
		return adapters.ConnectInfo{}, mapError(err)
	}
	a.setConnected(cl, adm, opts, cfg.ReadOnly)
	return adapters.ConnectInfo{
		ServerVersion: "Kafka",
		Details: map[string]string{
			"brokers": strconv.Itoa(len(meta.Brokers)),
			"cluster": meta.Cluster,
		},
	}, nil
}

func (a *Adapter) Disconnect(ctx context.Context) error {
	if client := a.state.Load().client; client != nil {
		client.Close()
	}
	a.clearConnected()
	return nil
}

func (a *Adapter) requireAdmin() (*kadm.Client, error) {
	return adapters.RequireConnected(a.state.Load().admin)
}

// Children is index.ts's children. Root is topics ∪ consumer groups (catalog.go's listRoot); a
// consumerGroup root returns [] (Adapter rule 5 — a leaf, never an error); a topic root one level
// down enumerates partitions (still needed even though the tree no longer expands a topic —
// StreamView.vue's partition filter popover is a second, live caller); anything deeper is a leaf.
func (a *Adapter) Children(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (adapters.TreeChildren, error) {
	adm, err := a.requireAdmin()
	if err != nil {
		return adapters.TreeChildren{}, err
	}
	segments := path.Segments
	if len(segments) == 0 {
		nodes, err := listRoot(ctx, adm)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		return adapters.TreeChildren{Nodes: nodes}, nil
	}

	root := segments[0]
	if root.Kind == "consumerGroup" {
		return adapters.TreeChildren{Nodes: []model.TreeNode{}}, nil
	}
	if root.Kind != "topic" {
		return adapters.TreeChildren{}, adapters.UnexpectedPathKind(0, root.Kind)
	}
	if len(segments) == 1 {
		nodes, err := listPartitions(ctx, adm, root.Name)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		return adapters.TreeChildren{Nodes: nodes}, nil
	}
	return adapters.TreeChildren{Nodes: []model.TreeNode{}}, nil // a partition — leaf.
}

// Describe is index.ts's describe — caps.Describe is false (P31 D2); unreachable while that flag
// gates every caller.
func (a *Adapter) Describe(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectMeta, error) {
	return model.ObjectMeta{}, adapters.Unsupported("kafka", "describe")
}

// SchemaColumns — caps.SchemaColumns is false; unreachable. A topic/consumer group has no
// column/PK/FK metadata.
func (a *Adapter) SchemaColumns(ctx context.Context, path model.NodePath, op *adapters.OpCtx) ([]model.RelationColumns, error) {
	return nil, adapters.Unsupported("kafka", "schemaColumns")
}

// Definition is index.ts's definition.
func (a *Adapter) Definition(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectDefinition, error) {
	adm, err := a.requireAdmin()
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	if len(path.Segments) == 0 {
		return model.ObjectDefinition{}, adapters.New(adapters.CodeNotFound,
			"definition requires a topic or consumer group path, got: "+model.EncodePath(path.Segments), nil)
	}
	switch path.Segments[0].Kind {
	case "topic":
		return buildTopicDefinition(ctx, adm, path.Segments[0].Name)
	case "consumerGroup":
		return buildGroupDefinition(ctx, adm, path.Segments[0].Name)
	default:
		return model.ObjectDefinition{}, adapters.New(adapters.CodeNotFound,
			"definition requires a topic or consumer group path, got: "+model.EncodePath(path.Segments), nil)
	}
}

// resolveTopicTarget is Read/Count/Preview/Mutate's shared single-topic path check (P113 G2). Every
// call site passes what="read" regardless of its own actual op — a pre-existing quirk kept as-is,
// out of this finding's scope. RequirePath's exact-length match also newly rejects a topic path
// with trailing segments past the root, which the old len==0 check silently allowed through: a topic
// is a tree leaf (Children never reports children for one), so no real caller ever sends a deeper
// path here.
func (a *Adapter) resolveTopicTarget(path model.NodePath, what string) (string, error) {
	segs, err := adapters.RequirePath(path, what, adapters.Seg("topic"))
	if err != nil {
		return "", err
	}
	return segs[0].Name, nil
}

// Read is index.ts's read.
func (a *Adapter) Read(ctx context.Context, req adapters.ReadRequest, op *adapters.OpCtx) (page.Page, error) {
	if err := adapters.RejectConditions("kafka", req.Conditions); err != nil {
		return nil, err
	}
	admin, err := a.requireAdmin()
	if err != nil {
		return nil, err
	}
	opts := a.state.Load().opts
	topic, err := a.resolveTopicTarget(req.Path, "read")
	if err != nil {
		return nil, err
	}
	return readTopic(ctx, admin, opts, topic, req, op)
}

// Count is index.ts's count.
func (a *Adapter) Count(ctx context.Context, req adapters.CountRequest, op *adapters.OpCtx) (adapters.CountResult, error) {
	adm, err := a.requireAdmin()
	if err != nil {
		return adapters.CountResult{}, err
	}
	topic, err := a.resolveTopicTarget(req.Path, "read")
	if err != nil {
		return adapters.CountResult{}, err
	}
	return countTopic(ctx, adm, topic, req.Filter)
}

// Preview is index.ts's preview.
func (a *Adapter) Preview(plan model.MutationPlan) ([]string, error) {
	topic, err := a.resolveTopicTarget(plan.Path, "read")
	if err != nil {
		return nil, err
	}
	return preview(plan, topic)
}

// Mutate is index.ts's mutate.
func (a *Adapter) Mutate(ctx context.Context, plan model.MutationPlan, op *adapters.OpCtx) (model.MutationResult, error) {
	client, err := adapters.RequireConnected(a.state.Load().client)
	if err != nil {
		return model.MutationResult{}, err
	}
	topic, err := a.resolveTopicTarget(plan.Path, "read")
	if err != nil {
		return model.MutationResult{}, err
	}
	return produce(ctx, client, topic, a.state.Load().readOnly, plan, op)
}

// Execute is index.ts's execute — caps.SQL is false (P10's D13); never reached.
func (a *Adapter) Execute(ctx context.Context, req model.ConsoleRequest, op *adapters.OpCtx) ([]page.Page, error) {
	return nil, adapters.NoQueryConsole("kafka")
}

// DownloadObject is index.ts's downloadObject — caps.FileTransfer is false; never reached.
func (a *Adapter) DownloadObject(ctx context.Context, req model.ObjectDownloadRequest, op *adapters.OpCtx) (model.ObjectTransferResult, error) {
	return model.ObjectTransferResult{}, adapters.Unsupported("kafka", "file transfer")
}

// KeyTypes — caps.KeyTypes is false; unreachable. A topic's messages have no per-item
// engine-level "type" the way a redis key does.
func (a *Adapter) KeyTypes(ctx context.Context, paths []model.NodePath, op *adapters.OpCtx) ([]string, error) {
	return nil, adapters.Unsupported("kafka", "key types")
}

// Cancel is index.ts's cancel — a permanent no-op. Unlike a sibling that
// carries adapters.RunWithAbortRace, Kafka has no server-side kill mechanism at all — a fetch is a
// request the broker answers or times out, there is no equivalent of pg_cancel_backend or KILL
// QUERY. The op's own context.Context, passed directly to every kadm/kgo call, is the
// entire cancellation story; it is confirmed to abort an in-flight PollRecords promptly.
// caps.Cancel stays true because that mechanism is real, even though this RPC itself reports
// nothing to forward.
func (a *Adapter) Cancel(ctx context.Context, opID string) (bool, error) {
	return false, nil
}
