package dbmcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// KubeReader is dbmcp's consumer-declared view of *kubernetes.Service.
type KubeReader interface {
	Clusters(context.Context) (kubernetes.ClustersResult, error)
	Cluster(ctx context.Context, id string) (kubernetes.Cluster, bool, error)
	Namespaces(context.Context, kubernetes.ClusterArgs) (kubernetes.NamespacesResult, error)
	Kinds(context.Context, kubernetes.ClusterArgs) (kubernetes.KindsResult, error)
	ListCapped(context.Context, kubernetes.ListArgs, int) (kubernetes.ListResult, error)
	Describe(context.Context, kubernetes.DescribeArgs) (kubernetes.Description, error)
	ReadLogs(context.Context, kubernetes.LogsReadArgs) (kubernetes.LogsReadResult, error)
}

const (
	kubeListDefaultRows = 200
	kubeListMaxRows     = 2000
	kubeLogsDefaultTail = 200
	kubeLogsMaxTail     = 5000
	kubeManifestMax     = 256 << 10
)

// Operation names, shown on the approval card.
const (
	kubeDiscover = "discover"
	kubeList     = "list"
	kubeDescribe = "describe"
	kubeLogs     = "logs"
)

type kubeClusterArgs struct {
	ClusterID string `json:"clusterId"`
}

type kubeListArgs struct {
	ClusterID     string          `json:"clusterId"`
	Kind          string          `json:"kind"`
	Namespace     string          `json:"namespace,omitempty"`
	LabelSelector string          `json:"labelSelector,omitempty"`
	MaxRows       int             `json:"maxRows,omitempty"`
	Script        *kubeScriptArgs `json:"script,omitempty"`
}

type kubeDescribeArgs struct {
	ClusterID string          `json:"clusterId"`
	Kind      string          `json:"kind"`
	Name      string          `json:"name"`
	Namespace string          `json:"namespace,omitempty"`
	Script    *kubeScriptArgs `json:"script,omitempty"`
}

type kubePodLogsArgs struct {
	ClusterID  string          `json:"clusterId"`
	Namespace  string          `json:"namespace"`
	Pod        string          `json:"pod"`
	Container  string          `json:"container,omitempty"`
	Previous   bool            `json:"previous,omitempty"`
	Timestamps bool            `json:"timestamps,omitempty"`
	Tail       int             `json:"tail,omitempty"`
	Script     *kubeScriptArgs `json:"script,omitempty"`
}

type kubeClusterView struct {
	ID          string `json:"id"`
	Context     string `json:"context"`
	Cluster     string `json:"cluster"`
	Server      string `json:"server"`
	Namespace   string `json:"namespace"`
	Current     bool   `json:"current"`
	Description string `json:"description"`
	Approval    string `json:"approval"`
	// ReviewResults says list, describe and logs results are held for the user before the AI gets them.
	ReviewResults bool `json:"reviewResults"`
}

type kubeKindView struct {
	Key        string `json:"key"`
	Kind       string `json:"kind"`
	Namespaced bool   `json:"namespaced"`
	Category   string `json:"category"`
}

type kubeRowView struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Cells     []string `json:"cells"`
}

type kubeDescribeResult struct {
	kubernetes.Description
	ManifestTruncated bool `json:"manifestTruncated"`
}

// kubeExposed reports whether the user turned the cluster on; an off cluster is hidden, like an
// unexposed connection.
func kubeExposed(m kubernetes.McpSettings) bool { return m.Enabled }

// resolveCluster finds an exposed cluster by id; found is false for an unknown or off one.
func (s *Server) resolveCluster(ctx context.Context, id string) (c kubernetes.Cluster, found bool, err error) {
	c, found, err = s.cfg.Kube.Cluster(ctx, id)
	if err != nil || !found || !kubeExposed(c.Mcp) {
		return kubernetes.Cluster{}, false, err
	}
	return c, true, nil
}

func noClusterResult(id string) (*mcp.CallToolResult, any, error) {
	return errResult(fmt.Sprintf("no cluster %q exposed to MCP", id))
}

// kubeReviewFor reports whether the cluster's list, describe and logs results are held for the user: the
// cluster's own setting, or the app's when it inherits.
func (s *Server) kubeReviewFor(c kubernetes.Cluster) bool {
	return reviewResolver(s.cfg.KubeReviewResponses)(c.Mcp.Review)
}

// kubeReviewable says whether an operation returns data from the cluster that the user can review. Discover
// returns namespaces and kinds only.
func kubeReviewable(op string) bool { return op != kubeDiscover }

// kubeRun is the gate every cluster read passes: the scripts, exposure, the prepare script, the approval for
// prompt, the read, the transform script, then the review of its result when the cluster, the app or the
// approver asks for one, and last the values the transform kept. The kubernetes service records the op-log
// row of the read.
func (s *Server) kubeRun(ctx context.Context, call kubeCall, statement func(kubernetes.Cluster, kubeRequest) string, read func(context.Context, kubeRequest) (any, error)) (*mcp.CallToolResult, any, error) {
	sc, refusal := s.openKubeScripts(ctx, call.tool, call.script)
	if refusal != "" {
		return errResult(refusal)
	}
	// Staged values are held until the final point; every way out but that one drops them.
	defer sc.dropKept()
	c, found, err := s.resolveCluster(ctx, call.clusterID)
	if err != nil {
		return toolError(err)
	}
	if !found {
		return noClusterResult(call.clusterID)
	}
	req := call.req
	if sc != nil && sc.Prepare != "" {
		var text string
		if req, text = s.prepareKube(ctx, call, sc); text != "" {
			return errResult(text)
		}
	}
	for _, key := range kubeRequired[call.op] {
		if strings.TrimSpace(req.text(key)) == "" {
			return errResult(key + " cannot be empty. " + kubeNothingRead)
		}
	}
	reviewable := kubeReviewable(call.op)
	areq := ApprovalRequest{
		ConnectionID: c.ID, ConnectionName: c.Context, Kind: "kubernetes",
		Class: adapters.ClassRead, Statement: statement(c, req), Reason: ApprovalReasonPermission,
		Cluster: &ApprovalCluster{Context: c.Context, Server: c.Server, Namespace: req.Namespace, Operation: call.op, Tool: call.tool, Request: req, Script: sc},
		Kept:    sc.keptUses(),
		Origin:  originFrom(ctx), OfferReview: reviewable, ReviewOn: reviewable && s.kubeReviewFor(c),
	}
	choice, exit := s.kubeApprove(ctx, call, c, areq, sc)
	if exit != nil {
		return exit.res, exit.out, exit.err
	}

	out, err := read(ctx, req)
	if err != nil {
		return toolError(err)
	}
	res, o, err := jsonResult(out)
	if err != nil || !reviewable {
		return res, o, err
	}
	return s.kubeFinish(ctx, call, c, req, areq, choice, sc, res, o, err)
}

// kubeExit is a tool reply that ends kubeRun early.
type kubeExit struct {
	res *mcp.CallToolResult
	out any
	err error
}

func kubeExitOf(res *mcp.CallToolResult, out any, err error) *kubeExit {
	return &kubeExit{res: res, out: out, err: err}
}

// kubeApprove asks the user to approve the read unless the cluster allows it outright. A non-nil exit ends
// the call.
func (s *Server) kubeApprove(ctx context.Context, call kubeCall, c kubernetes.Cluster, areq ApprovalRequest, sc *apiScripts) (ApprovalChoice, *kubeExit) {
	var choice ApprovalChoice
	// Fail closed: anything but "allow" asks.
	if c.Mcp.Approval == "allow" {
		return choice, nil
	}
	wctx, stop := s.watchKept(ctx, sc.keptUses())
	outcome, ch := s.cfg.Approvals.RequestChoice(wctx, areq)
	cause := context.Cause(wctx)
	stop()
	if outcome != ApprovalApproved {
		var gone *keptGoneError
		if errors.As(cause, &gone) {
			return choice, kubeExitOf(errResult(gone.kubeText()))
		}
		return choice, kubeExitOf(approvalOutcomeError(outcome, fmt.Sprintf("Kubernetes read on %q", c.Context)))
	}
	// The wait can last up to ApprovalTimeout: a cluster turned off meanwhile wins.
	_, found, err := s.resolveCluster(ctx, call.clusterID)
	if err != nil {
		return choice, kubeExitOf(toolError(err))
	}
	if !found {
		return choice, kubeExitOf(noClusterResult(call.clusterID))
	}
	if sc != nil && !s.scriptsOn() {
		return choice, kubeExitOf(errResult(apiScriptsOff + ". " + kubeNothingRead))
	}
	return ch, nil
}

// kubeFinish runs the transform script, the result review and the kept-value commit on a read result.
func (s *Server) kubeFinish(ctx context.Context, call kubeCall, c kubernetes.Cluster, req kubeRequest, areq ApprovalRequest, choice ApprovalChoice, sc *apiScripts, res *mcp.CallToolResult, o any, err error) (*mcp.CallToolResult, any, error) {
	var so *kubeScriptOutcome
	if sc.hasTransform() {
		so = s.transformKube(ctx, call, req, sc, resultText(res))
		res, o, err = so.agentResult(nil, nil)
	}
	// Read after the read: the user may have changed the setting while it ran. A cluster that is gone now
	// is held and withheld by the recheck.
	fresh, found, ferr := s.resolveCluster(ctx, call.clusterID)
	if choice.ReviewResult || ferr != nil || !found || s.kubeReviewFor(fresh) {
		held := ApprovalResponse{IsError: res.IsError, Text: resultText(res), Requested: choice.ReviewResult}
		held.Bytes = len(held.Text)
		if so != nil {
			held = so.heldOf(res, choice.ReviewResult)
		}
		if refusal := s.holdKube(ctx, areq, c, call.op, held, so != nil); refusal != "" {
			return errResult(refusal)
		}
	}
	if so != nil && so.Failure == "" && ctx.Err() == nil {
		if saved, failed := sc.kept.commit(keptSourceOfKube(ctx, c, call.tool, areq.Statement), ""); len(saved)+len(failed) > 0 {
			return so.agentResult(saved, failed)
		}
	}
	return res, o, err
}

// holdKube holds the result of a cluster read until the user sends or withholds it. A non-empty string is
// the refusal text; "" means the user sent the result and it goes out untouched. req is the pre-read request,
// whose cluster and statement name the call.
func (s *Server) holdKube(ctx context.Context, req ApprovalRequest, c kubernetes.Cluster, op string, held ApprovalResponse, scripted bool) string {
	call := kubeHeldCall(c.Context, op)
	req.OfferReview, req.ReviewOn = false, false
	return s.holdResult(ctx, req, held, call, func() string {
		// The review can take up to ApprovalTimeout: a cluster turned off meanwhile wins.
		if _, found, err := s.resolveCluster(ctx, c.ID); err != nil || !found {
			return call.suffix(call.ran + ", but the cluster was turned off while its result was in review, so it was withheld.")
		}
		if scripted && !s.scriptsOn() {
			return call.suffix(call.ran + ", but scripts were turned off while its result was in review, so it was withheld.")
		}
		return ""
	})
}

func (s *Server) kubeListClusters(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	res, err := s.cfg.Kube.Clusters(ctx)
	if err != nil {
		return toolError(err)
	}
	out := make([]kubeClusterView, 0, len(res.Clusters))
	for _, c := range res.Clusters {
		if !kubeExposed(c.Mcp) {
			continue
		}
		out = append(out, kubeClusterView{
			ID: c.ID, Context: c.Context, Cluster: c.Cluster, Server: c.Server, Namespace: c.Namespace,
			Current: c.Current, Description: c.Mcp.Description,
			Approval: c.Mcp.Approval, ReviewResults: s.kubeReviewFor(c),
		})
	}
	return jsonResult(out)
}

// kubeDiscoverResult is kube_discover's answer: the namespaces and the listable kinds of one cluster.
type kubeDiscoverResult struct {
	kubernetes.NamespacesResult
	Kinds   []kubeKindView `json:"kinds"`
	Partial bool           `json:"partial"`
}

// kubeDiscoverTool returns namespaces and kinds under one discover approval.
func (s *Server) kubeDiscoverTool(ctx context.Context, _ *mcp.CallToolRequest, a kubeClusterArgs) (*mcp.CallToolResult, any, error) {
	return s.kubeRun(ctx, kubeCall{tool: "kube_discover", op: kubeDiscover, clusterID: a.ClusterID},
		func(c kubernetes.Cluster, _ kubeRequest) string {
			return kubernetes.NamespacesCommand(c.Context) + "\n" + kubernetes.KindsCommand(c.Context)
		},
		func(ctx context.Context, _ kubeRequest) (any, error) {
			ns, err := s.cfg.Kube.Namespaces(ctx, kubernetes.ClusterArgs{ClusterID: a.ClusterID})
			if err != nil {
				return nil, err
			}
			r, err := s.cfg.Kube.Kinds(ctx, kubernetes.ClusterArgs{ClusterID: a.ClusterID})
			if err != nil {
				return nil, err
			}
			kinds := make([]kubeKindView, 0, len(r.Kinds))
			for _, k := range r.Kinds {
				kinds = append(kinds, kubeKindView{Key: k.Key, Kind: k.Kind, Namespaced: k.Namespaced, Category: k.Category})
			}
			return kubeDiscoverResult{NamespacesResult: ns, Kinds: kinds, Partial: r.Partial}, nil
		})
}

func (s *Server) kubeListResources(ctx context.Context, _ *mcp.CallToolRequest, a kubeListArgs) (*mcp.CallToolResult, any, error) {
	call := kubeCall{
		tool: "kube_list_resources", op: kubeList, clusterID: a.ClusterID, script: a.Script,
		req: kubeClamp(kubeList, kubeRequest{Kind: a.Kind, Namespace: a.Namespace, LabelSelector: a.LabelSelector, MaxRows: a.MaxRows}),
	}
	return s.kubeRun(ctx, call,
		func(c kubernetes.Cluster, r kubeRequest) string {
			return kubernetes.ListCommand(c.Context, kubernetes.ListArgs{ClusterID: a.ClusterID, Kind: r.Kind, Namespace: r.Namespace, LabelSelector: r.LabelSelector})
		},
		func(ctx context.Context, r kubeRequest) (any, error) {
			res, err := s.cfg.Kube.ListCapped(ctx, kubernetes.ListArgs{
				ClusterID: a.ClusterID, Kind: r.Kind, Namespace: r.Namespace, LabelSelector: r.LabelSelector,
			}, r.MaxRows)
			if err != nil {
				return nil, err
			}
			cols := make([]string, 0, len(res.Columns))
			for _, col := range res.Columns {
				cols = append(cols, col.Name)
			}
			rows := make([]kubeRowView, 0, len(res.Rows))
			for _, row := range res.Rows {
				rows = append(rows, kubeRowView{Name: row.Name, Namespace: row.Namespace, Cells: row.Cells})
			}
			return map[string]any{"columns": cols, "rows": rows, "rowCount": len(rows), "truncated": res.Truncated}, nil
		})
}

func (s *Server) kubeDescribeTool(ctx context.Context, _ *mcp.CallToolRequest, a kubeDescribeArgs) (*mcp.CallToolResult, any, error) {
	call := kubeCall{
		tool: "kube_describe", op: kubeDescribe, clusterID: a.ClusterID, script: a.Script,
		req: kubeRequest{Kind: a.Kind, Name: a.Name, Namespace: a.Namespace},
	}
	return s.kubeRun(ctx, call,
		func(c kubernetes.Cluster, r kubeRequest) string {
			return kubernetes.DescribeCommand(c.Context, kubernetes.DescribeArgs{ClusterID: a.ClusterID, Kind: r.Kind, Namespace: r.Namespace, Name: r.Name})
		},
		func(ctx context.Context, r kubeRequest) (any, error) {
			d, err := s.cfg.Kube.Describe(ctx, kubernetes.DescribeArgs{
				ClusterID: a.ClusterID, Kind: r.Kind, Namespace: r.Namespace, Name: r.Name,
			})
			if err != nil {
				return nil, err
			}
			res := kubeDescribeResult{Description: d}
			res.Manifest, res.ManifestTruncated = cutLines(d.Manifest, kubeManifestMax)
			return res, nil
		})
}

func (s *Server) kubePodLogs(ctx context.Context, _ *mcp.CallToolRequest, a kubePodLogsArgs) (*mcp.CallToolResult, any, error) {
	call := kubeCall{
		tool: "kube_pod_logs", op: kubeLogs, clusterID: a.ClusterID, script: a.Script,
		req: kubeClamp(kubeLogs, kubeRequest{Namespace: a.Namespace, Pod: a.Pod, Container: a.Container, Previous: a.Previous, Timestamps: a.Timestamps, Tail: a.Tail}),
	}
	return s.kubeRun(ctx, call,
		func(c kubernetes.Cluster, r kubeRequest) string {
			return kubernetes.LogsCommand(c.Context, kubernetes.LogsReadArgs{Namespace: r.Namespace, Pod: r.Pod, Container: r.Container, Previous: r.Previous, Timestamps: r.Timestamps, Tail: r.Tail}, false)
		},
		func(ctx context.Context, r kubeRequest) (any, error) {
			return s.cfg.Kube.ReadLogs(ctx, kubernetes.LogsReadArgs{
				ClusterID: a.ClusterID, Namespace: r.Namespace, Pod: r.Pod, Container: r.Container,
				Previous: r.Previous, Timestamps: r.Timestamps, Tail: r.Tail,
			})
		})
}

func clampOr(v, def, max int) int {
	switch {
	case v <= 0:
		return def
	case v > max:
		return max
	}
	return v
}

// cutLines shortens s to at most max bytes on a line boundary.
func cutLines(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	cut := s[:max]
	if i := strings.LastIndexByte(cut, '\n'); i >= 0 {
		cut = cut[:i+1]
	}
	return cut, true
}

// addKubeTools registers the five read-only kube tools next to the database tools.
func (s *Server) addKubeTools(srv *mcp.Server, src llmprompts.Source) {
	desc := func(e llmprompts.Prompt[llmprompts.NoVars]) string { return e.Text(src, llmprompts.NoVars{}) }
	scripts := s.scriptsOn()
	// The three tools that return cluster data carry the review notice and, while scripts are on, the script one.
	reviewed := func(e llmprompts.Prompt[llmprompts.NoVars]) string {
		text := desc(e) + " " + desc(llmprompts.DbmcpToolKubeReview)
		if scripts {
			text += " " + desc(llmprompts.DbmcpToolKubeScripts)
		}
		return text
	}
	// dataSchema pins script to an object and drops it while scripts are off. A prepare can set the
	// required args, so with scripts on they may be omitted; kubeRun enforces them on the final request.
	dataSchema := func(op string, sch *jsonschema.Schema) *jsonschema.Schema {
		if scripts {
			pinScriptObject(sch)
			sch.Required = slices.DeleteFunc(sch.Required, func(n string) bool { return slices.Contains(kubeRequired[op], n) })
		} else {
			delete(sch.Properties, "script")
		}
		return sch
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name: "kube_list_clusters", Description: desc(llmprompts.DbmcpToolKubeListClusters),
	}, withPanicRecovery(s, "kube_list_clusters", s.kubeListClusters))
	mcp.AddTool(srv, &mcp.Tool{
		Name: "kube_discover", Description: desc(llmprompts.DbmcpToolKubeDiscover),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[kubeClusterArgs](src, llmprompts.DbmcpToolKubeDiscover.ID(), nil), "clusterId"),
	}, withPanicRecovery(s, "kube_discover", s.kubeDiscoverTool))
	mcp.AddTool(srv, &mcp.Tool{
		Name: "kube_list_resources", Description: reviewed(llmprompts.DbmcpToolKubeListResources),
		InputSchema: dataSchema(kubeList, llmprompts.RequireNonEmpty(llmprompts.InputSchema[kubeListArgs](src, llmprompts.DbmcpToolKubeListResources.ID(),
			map[string]any{"maxRows": llmprompts.LimitVars{Default: kubeListDefaultRows, Max: kubeListMaxRows}}), "clusterId", "kind")),
	}, withPanicRecovery(s, "kube_list_resources", s.kubeListResources))
	mcp.AddTool(srv, &mcp.Tool{
		Name: "kube_describe", Description: reviewed(llmprompts.DbmcpToolKubeDescribe),
		InputSchema: dataSchema(kubeDescribe, llmprompts.RequireNonEmpty(llmprompts.InputSchema[kubeDescribeArgs](src, llmprompts.DbmcpToolKubeDescribe.ID(), nil), "clusterId", "kind", "name")),
	}, withPanicRecovery(s, "kube_describe", s.kubeDescribeTool))
	mcp.AddTool(srv, &mcp.Tool{
		Name: "kube_pod_logs", Description: reviewed(llmprompts.DbmcpToolKubePodLogs),
		InputSchema: dataSchema(kubeLogs, llmprompts.RequireNonEmpty(llmprompts.InputSchema[kubePodLogsArgs](src, llmprompts.DbmcpToolKubePodLogs.ID(),
			map[string]any{"tail": llmprompts.LimitVars{Default: kubeLogsDefaultTail, Max: kubeLogsMaxTail}}), "clusterId", "namespace", "pod")),
	}, withPanicRecovery(s, "kube_pod_logs", s.kubePodLogs))
}
