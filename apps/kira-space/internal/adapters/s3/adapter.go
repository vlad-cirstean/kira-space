package s3

import (
	"context"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func init() {
	adapters.Register("s3", func(deps adapters.Deps) (adapters.Adapter, error) {
		return &Adapter{deps: deps}, nil
	})
}

// connState is every field Connect/Disconnect write concurrently with an in-flight op reading them
// (F3), guarded together via adapters.Guarded (P113 G1).
type connState struct {
	client *awss3.Client
	// scopedBucket (options.bucket) — set, this scopes the whole tree to one bucket, for
	// credentials that can only ever see that one bucket.
	scopedBucket string
	readOnly     bool
}

// Adapter is index.ts's S3Adapter. Mirrors redis's adapter closely: bucket ~ redis's database,
// prefix ~ redis's namespace, object ~ redis's key — a lazy, '/'-delimited key space. Only the
// bucket itself is a project-tree row; prefix/object stay reachable exclusively through the
// Browse tab, which calls the same Children this struct has always exposed.
type Adapter struct {
	deps adapters.Deps

	state adapters.Guarded[connState]
}

// setConnected is Connect's own locked write of every field a successful connect fills in (F3).
func (a *Adapter) setConnected(client *awss3.Client, scopedBucket string, readOnly bool) {
	a.state.Update(func(s *connState) {
		s.client = client
		s.scopedBucket = scopedBucket
		s.readOnly = readOnly
	})
}

// clearConnected is Disconnect's own locked write (F3).
func (a *Adapter) clearConnected() {
	a.state.Update(func(s *connState) {
		s.client = nil
		s.scopedBucket = ""
	})
}

func (a *Adapter) Kind() string        { return "s3" }
func (a *Adapter) Caps() adapters.Caps { return caps }

// Connect is index.ts's connect.
func (a *Adapter) Connect(ctx context.Context, cfg model.ResolvedConnectionConfig, op *adapters.OpCtx) (adapters.ConnectInfo, error) {
	client, err := connect(ctx, cfg, a.deps.Log)
	if err != nil {
		return adapters.ConnectInfo{}, err
	}
	scopedBucket := ""
	if raw, ok := cfg.Options["bucket"]; ok {
		if s, ok := raw.(string); ok && s != "" {
			scopedBucket = s
		}
	}
	if _, err := listBuckets(ctx, client, scopedBucket); err != nil {
		return adapters.ConnectInfo{}, err
	}
	a.setConnected(client, scopedBucket, cfg.ReadOnly)
	return adapters.ConnectInfo{ServerVersion: "Amazon S3"}, nil
}

// Disconnect is index.ts's disconnect.
func (a *Adapter) Disconnect(ctx context.Context) error {
	a.clearConnected()
	return nil
}

// enforceScope refuses any bucket other than options.bucket: the tree honours the option, so
// crafted paths (an MCP agent's list_children) must too.
func enforceScope(scopedBucket, bucket string) error {
	if scopedBucket != "" && bucket != scopedBucket {
		return adapters.New(adapters.CodeNotFound, "bucket not found: "+bucket, nil)
	}
	return nil
}

func (a *Adapter) requireClient() (*awss3.Client, error) {
	return adapters.RequireConnected(a.state.Load().client)
}

// Children is index.ts's children.
func (a *Adapter) Children(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (adapters.TreeChildren, error) {
	client, err := a.requireClient()
	if err != nil {
		return adapters.TreeChildren{}, err
	}
	segments := path.Segments
	if len(segments) == 0 {
		nodes, err := listBuckets(ctx, client, a.state.Load().scopedBucket)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		return adapters.TreeChildren{Nodes: nodes}, nil
	}

	bucketSegment := segments[0]
	if bucketSegment.Kind != "bucket" {
		return adapters.TreeChildren{}, adapters.UnexpectedPathKind(0, bucketSegment.Kind)
	}
	if err := enforceScope(a.state.Load().scopedBucket, bucketSegment.Name); err != nil {
		return adapters.TreeChildren{}, err
	}
	rest := segments[1:]
	// Rule 5 (Adapter doc comment): Children returns [] for a leaf, never an error — an 'object'
	// node never has children.
	if len(rest) > 0 && rest[len(rest)-1].Kind == "object" {
		return adapters.TreeChildren{Nodes: []model.TreeNode{}}, nil
	}

	prefixSegments := make([]string, 0, len(rest))
	for _, seg := range rest {
		if seg.Kind != "prefix" {
			return adapters.TreeChildren{}, adapters.New(adapters.CodeNotFound, "unexpected path segment kind: "+seg.Kind, nil)
		}
		prefixSegments = append(prefixSegments, seg.Name)
	}
	return listPrefixChildren(ctx, client, bucketSegment.Name, prefixSegments, op)
}

// Describe is index.ts's describe — caps.Describe is false; unreachable while that flag gates
// every caller.
func (a *Adapter) Describe(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectMeta, error) {
	return model.ObjectMeta{}, adapters.Unsupported("s3", "describe")
}

// Definition is index.ts's definition — caps.Definition is false; unreachable.
func (a *Adapter) Definition(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectDefinition, error) {
	return model.ObjectDefinition{}, adapters.Unsupported("s3", "definition")
}

// SchemaColumns — caps.SchemaColumns is false; unreachable. An S3 object has no column-shaped
// schema at all.
func (a *Adapter) SchemaColumns(ctx context.Context, path model.NodePath, op *adapters.OpCtx) ([]model.RelationColumns, error) {
	return nil, adapters.Unsupported("s3", "schemaColumns")
}

func (a *Adapter) resolveObjectTarget(path model.NodePath) (bucket, key string, err error) {
	segments := path.Segments
	if len(segments) == 0 {
		return "", "", adapters.New(adapters.CodeNotFound, "read requires a bucket/.../object path, got: "+model.EncodePath(segments), nil)
	}
	bucketSegment := segments[0]
	rest := segments[1:]
	if len(rest) == 0 {
		return "", "", adapters.New(adapters.CodeNotFound, "read requires a bucket/.../object path, got: "+model.EncodePath(segments), nil)
	}
	objectSegment := rest[len(rest)-1]
	if bucketSegment.Kind != "bucket" || objectSegment.Kind != "object" {
		return "", "", adapters.New(adapters.CodeNotFound, "read requires a bucket/.../object path, got: "+model.EncodePath(segments), nil)
	}
	if err := enforceScope(a.state.Load().scopedBucket, bucketSegment.Name); err != nil {
		return "", "", err
	}
	// objectSegment.Name is already the full key (catalog.go encodes it that way) — no
	// prefix-segment joining needed.
	return bucketSegment.Name, objectSegment.Name, nil
}

// Read is index.ts's read.
func (a *Adapter) Read(ctx context.Context, req adapters.ReadRequest, op *adapters.OpCtx) (page.Page, error) {
	if err := adapters.RejectConditions("s3", req.Conditions); err != nil {
		return nil, err
	}
	client, err := a.requireClient()
	if err != nil {
		return nil, err
	}
	bucket, key, err := a.resolveObjectTarget(req.Path)
	if err != nil {
		return nil, err
	}
	return readObject(ctx, client, bucket, key, op)
}

// Count is index.ts's count.
func (a *Adapter) Count(ctx context.Context, req adapters.CountRequest, op *adapters.OpCtx) (adapters.CountResult, error) {
	client, err := a.requireClient()
	if err != nil {
		return adapters.CountResult{}, err
	}
	bucket, key, err := a.resolveObjectTarget(req.Path)
	if err != nil {
		return adapters.CountResult{}, err
	}
	return countObject(ctx, client, bucket, key, op)
}

// Preview is index.ts's preview.
func (a *Adapter) Preview(plan model.MutationPlan) ([]string, error) {
	return preview(plan)
}

// Mutate is index.ts's mutate. A single client serves the whole bucket-rooted tree (unlike
// mariadb/postgres's per-database connection set) — mutate.go's own resolveBucketSegment validates
// plan.Path, so this just forwards the client and the connection's read-only flag.
func (a *Adapter) Mutate(ctx context.Context, plan model.MutationPlan, op *adapters.OpCtx) (model.MutationResult, error) {
	client, err := a.requireClient()
	if err != nil {
		return model.MutationResult{}, err
	}
	st := a.state.Load()
	return mutate(ctx, client, op, st.readOnly, st.scopedBucket, plan, a.deps.Log)
}

// Execute is index.ts's execute — caps.SQL is false; never reached.
func (a *Adapter) Execute(ctx context.Context, req model.ConsoleRequest, op *adapters.OpCtx) ([]page.Page, error) {
	return nil, adapters.NoQueryConsole("s3")
}

// DownloadObject is index.ts's downloadObject — forwards to transfer.go without AssertWritable: a
// download is a read, never blocked by the connection's read-only flag (Adapter interface's own
// DownloadObject doc comment).
func (a *Adapter) DownloadObject(ctx context.Context, req model.ObjectDownloadRequest, op *adapters.OpCtx) (model.ObjectTransferResult, error) {
	client, err := a.requireClient()
	if err != nil {
		return model.ObjectTransferResult{}, err
	}
	bucket, key, err := a.resolveObjectTarget(req.Path)
	if err != nil {
		return model.ObjectTransferResult{}, err
	}
	return downloadObject(ctx, client, bucket, key, req.DestPath, op)
}

// KeyTypes — caps.KeyTypes is false; unreachable. An S3 object has no per-item engine-level
// "type" the way a redis key's TYPE command reports one (caps.go's own comment).
func (a *Adapter) KeyTypes(ctx context.Context, paths []model.NodePath, op *adapters.OpCtx) ([]string, error) {
	return nil, adapters.Unsupported("s3", "key types")
}

// Cancel is index.ts's cancel — a permanent no-op, for the identical reason sqs's own Cancel
// names: no server-side kill mechanism exists, so the op's own context.Context, passed directly
// to every SDK call in catalog.go/read.go/mutate.go/transfer.go (never
// adapters.RunWithAbortRace), is the entire cancellation story. Additionally load-bearing for
// DownloadObject's temp-file cleanup ordering (transfer.go).
func (a *Adapter) Cancel(ctx context.Context, opID string) (bool, error) {
	return false, nil
}
