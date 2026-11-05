package sqs

import (
	"context"

	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

func init() {
	adapters.Register("sqs", func(deps adapters.Deps) (adapters.Adapter, error) {
		return &Adapter{deps: deps}, nil
	})
}

// connState is every field Connect/Disconnect write concurrently with an in-flight op reading them,
// guarded together via adapters.Guarded. It also folds in the pre-existing "two
// tabs on one connection are two goroutines through one *Adapter in Go" guard the queueURLs cache
// already carried.
type connState struct {
	client   *awssqs.Client
	readOnly bool

	// queueURLs is P58d D9: name -> URL, populated by listQueues (free — it already has every URL
	// while paging) and by resolveQueueURL on a miss; avoids a GetQueueUrl round trip on every
	// read()/count() call.
	queueURLs map[string]string

	receiptHandles *receiptHandles
}

// Adapter is index.ts's SqsAdapter.
type Adapter struct {
	deps adapters.Deps

	state adapters.Guarded[connState]
}

func (a *Adapter) Kind() string        { return "sqs" }
func (a *Adapter) Caps() adapters.Caps { return caps }

// Connect is index.ts's connect. The SDK client owns no sockets to destroy on failure — unlike
// the TypeScript's client.destroy(), there is no Go counterpart, so a failed listQueues probe
// just returns the mapped error.
func (a *Adapter) Connect(ctx context.Context, cfg model.ResolvedConnectionConfig, op *adapters.OpCtx) (adapters.ConnectInfo, error) {
	client, err := connect(ctx, cfg, a.deps.Log)
	if err != nil {
		return adapters.ConnectInfo{}, err
	}
	if _, err := listQueues(ctx, client); err != nil {
		return adapters.ConnectInfo{}, err
	}
	a.state.Update(func(s *connState) {
		s.client = client
		s.readOnly = cfg.ReadOnly
		s.queueURLs = map[string]string{}
		s.receiptHandles = newReceiptHandles()
	})
	return adapters.ConnectInfo{ServerVersion: "Amazon SQS"}, nil
}

// Disconnect is index.ts's disconnect. Clears both adapter-local caches under the lock — the
// property checkpoint scenario 16 asserts.
func (a *Adapter) Disconnect(ctx context.Context) error {
	var handles *receiptHandles
	a.state.Update(func(s *connState) {
		s.client = nil
		s.queueURLs = nil
		handles = s.receiptHandles
		s.receiptHandles = nil
	})
	if handles != nil {
		handles.clear()
	}
	return nil
}

func (a *Adapter) requireClient() (*awssqs.Client, error) {
	return adapters.RequireConnected(a.state.Load().client)
}

// cacheQueueURL is a no-op once Disconnect has nilled queueURLs: an op already past
// requireClient can still reach this call (or Children's own listing loop) after a concurrent
// Disconnect, and writing to a nil map panics — recovered by safeRun into a bare E_INTERNAL before
// this fix, but a genuine bug regardless of the recover.
func (a *Adapter) cacheQueueURL(name, url string) {
	a.state.Update(func(s *connState) {
		if s.queueURLs != nil {
			s.queueURLs[name] = url
		}
	})
}

func (a *Adapter) cachedQueueURL(name string) (string, bool) {
	// View, not Load: queueURLs is mutated in place by cacheQueueURL (never reassigned wholesale
	// after Connect), so the index lookup must stay inside the same critical section as that write.
	var url string
	var ok bool
	a.state.View(func(s connState) { url, ok = s.queueURLs[name] })
	return url, ok
}

func (a *Adapter) resolveQueueURLCached(ctx context.Context, client *awssqs.Client, name string) (string, error) {
	if url, ok := a.cachedQueueURL(name); ok {
		return url, nil
	}
	url, err := resolveQueueURL(ctx, client, name)
	if err != nil {
		return "", err
	}
	a.cacheQueueURL(name, url)
	return url, nil
}

// Children is index.ts's children. Rule 5 (Adapter doc comment): returns [] for a leaf, never an
// error — a 'queue' node never has children (a flat "region -> queues" tree, no deeper level).
func (a *Adapter) Children(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (adapters.TreeChildren, error) {
	if len(path.Segments) > 0 {
		return adapters.TreeChildren{Nodes: []model.TreeNode{}}, nil
	}
	client, err := a.requireClient()
	if err != nil {
		return adapters.TreeChildren{}, err
	}
	listing, err := listQueues(ctx, client)
	if err != nil {
		return adapters.TreeChildren{}, err
	}
	for name, url := range listing.urlByName {
		a.cacheQueueURL(name, url)
	}
	return adapters.TreeChildren{Nodes: listing.nodes}, nil
}

// Describe is index.ts's describe — caps.Describe is false; unreachable while that flag gates
// every caller.
func (a *Adapter) Describe(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectMeta, error) {
	return model.ObjectMeta{}, adapters.Unsupported("sqs", "describe")
}

// SchemaColumns — caps.SchemaColumns is false; unreachable. A queue has no column/PK/FK metadata.
func (a *Adapter) SchemaColumns(ctx context.Context, path model.NodePath, op *adapters.OpCtx) ([]model.RelationColumns, error) {
	return nil, adapters.Unsupported("sqs", "schemaColumns")
}

// resolveQueueTarget is Definition/Read/Count/Preview/Mutate's shared single-queue path check.
// The message text stays hardcoded to "read" regardless of the actual caller, a
// pre-existing quirk kept as-is. A queue is a tree leaf, so RequirePath's exact-length match (vs.
// the old check's mere non-emptiness) changes nothing for any real caller.
func (a *Adapter) resolveQueueTarget(path model.NodePath) (string, error) {
	segs, err := adapters.RequirePath(path, "read", adapters.Seg("queue"))
	if err != nil {
		return "", err
	}
	return segs[0].Name, nil
}

// Definition is index.ts's definition.
func (a *Adapter) Definition(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectDefinition, error) {
	client, err := a.requireClient()
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	name, err := a.resolveQueueTarget(path)
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	url, err := a.resolveQueueURLCached(ctx, client, name)
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	return buildQueueDefinition(ctx, client, url, name)
}

// Read is index.ts's read.
func (a *Adapter) Read(ctx context.Context, req adapters.ReadRequest, op *adapters.OpCtx) (page.Page, error) {
	if err := adapters.RejectConditions("sqs", req.Conditions); err != nil {
		return nil, err
	}
	browseSeconds, err := parseStreamFilter(req.Filter)
	if err != nil {
		return nil, err
	}
	client, err := a.requireClient()
	if err != nil {
		return nil, err
	}
	name, err := a.resolveQueueTarget(req.Path)
	if err != nil {
		return nil, err
	}
	url, err := a.resolveQueueURLCached(ctx, client, name)
	if err != nil {
		return nil, err
	}
	return pollQueue(ctx, client, url, req, op, a.state.Load().receiptHandles, a.state.Load().readOnly, browseSeconds)
}

// Count is index.ts's count.
func (a *Adapter) Count(ctx context.Context, req adapters.CountRequest, op *adapters.OpCtx) (adapters.CountResult, error) {
	client, err := a.requireClient()
	if err != nil {
		return adapters.CountResult{}, err
	}
	name, err := a.resolveQueueTarget(req.Path)
	if err != nil {
		return adapters.CountResult{}, err
	}
	url, err := a.resolveQueueURLCached(ctx, client, name)
	if err != nil {
		return adapters.CountResult{}, err
	}
	return countQueue(ctx, client, url)
}

// Preview is index.ts's preview.
func (a *Adapter) Preview(plan model.MutationPlan) ([]string, error) {
	name, err := a.resolveQueueTarget(plan.Path)
	if err != nil {
		return nil, err
	}
	return preview(plan, name)
}

// Mutate is index.ts's mutate.
func (a *Adapter) Mutate(ctx context.Context, plan model.MutationPlan, op *adapters.OpCtx) (model.MutationResult, error) {
	client, err := a.requireClient()
	if err != nil {
		return model.MutationResult{}, err
	}
	name, err := a.resolveQueueTarget(plan.Path)
	if err != nil {
		return model.MutationResult{}, err
	}
	url, err := a.resolveQueueURLCached(ctx, client, name)
	if err != nil {
		return model.MutationResult{}, err
	}
	return mutateQueue(ctx, client, url, name, a.state.Load().readOnly, plan, a.state.Load().receiptHandles, op)
}

// Execute is index.ts's execute — caps.SQL is false; never reached.
func (a *Adapter) Execute(ctx context.Context, req model.ConsoleRequest, op *adapters.OpCtx) ([]page.Page, error) {
	return nil, adapters.NoQueryConsole("sqs")
}

// DownloadObject is index.ts's downloadObject — caps.FileTransfer is false; never reached.
func (a *Adapter) DownloadObject(ctx context.Context, req model.ObjectDownloadRequest, op *adapters.OpCtx) (model.ObjectTransferResult, error) {
	return model.ObjectTransferResult{}, adapters.Unsupported("sqs", "file transfer")
}

// KeyTypes — caps.KeyTypes is false; unreachable. A queue's messages have no per-item
// engine-level "type" the way a redis key does.
func (a *Adapter) KeyTypes(ctx context.Context, paths []model.NodePath, op *adapters.OpCtx) ([]string, error) {
	return nil, adapters.Unsupported("sqs", "key types")
}

// Cancel is index.ts's cancel — a permanent no-op. SQS has no server-side kill mechanism at all:
// the op's own context.Context, passed directly to every SDK call in read.go/mutate.go (never
// adapters.RunWithAbortRace), is the entire cancellation story. A cancelled context aborts an
// in-flight ReceiveMessage promptly through the SDK's own plumbing.
func (a *Adapter) Cancel(ctx context.Context, opID string) (bool, error) {
	return false, nil
}
