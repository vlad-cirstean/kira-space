package dbmcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// kubeReadArgs are the args of kube_read; op picks which of them apply.
type kubeReadArgs struct {
	ClusterID     string          `json:"clusterId"`
	Op            string          `json:"op"`
	Kind          string          `json:"kind,omitempty"`
	Namespace     string          `json:"namespace,omitempty"`
	Name          string          `json:"name,omitempty"`
	LabelSelector string          `json:"labelSelector,omitempty"`
	MaxRows       int             `json:"maxRows,omitempty"`
	Container     string          `json:"container,omitempty"`
	Previous      bool            `json:"previous,omitempty"`
	Timestamps    bool            `json:"timestamps,omitempty"`
	Tail          int             `json:"tail,omitempty"`
	Script        *kubeScriptArgs `json:"script,omitempty"`
	Confirm       string          `json:"confirm,omitempty"`
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
	ManifestTruncated bool   `json:"manifestTruncated,omitempty"`
	Hint              string `json:"hint,omitempty"`
}

// kubeLogsResult is the lines of a log read, with what to do when the cap cut them.
type kubeLogsResult struct {
	kubernetes.LogsReadResult
	Hint string `json:"hint,omitempty"`
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
			return errResult(kubeNeeds(call.op) + ". " + kubeNothingRead)
		}
	}
	reviewable := kubeReviewable(call.op)
	areq := ApprovalRequest{
		ConnectionID: c.ID, ConnectionName: c.Context, Kind: "kubernetes",
		Class: adapters.ClassRead, Statement: statement(c, req), Reason: confirmReason(c.Mcp.Approval != "allow"), AgentNote: call.confirm,
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

// kubeApprove asks the user to approve the read unless the cluster allows it outright and the agent did not
// confirm. A non-nil exit ends the call.
func (s *Server) kubeApprove(ctx context.Context, call kubeCall, c kubernetes.Cluster, areq ApprovalRequest, sc *apiScripts) (ApprovalChoice, *kubeExit) {
	var choice ApprovalChoice
	// Fail closed: anything but "allow" asks.
	if c.Mcp.Approval == "allow" && call.confirm == "" {
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
		res, o, err = so.agentResult(nil, nil, nil)
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
	if sc != nil && (so == nil || so.Failure == "") && ctx.Err() == nil {
		saved, failed, deleted := sc.kept.commit(keptSourceOfKube(ctx, c, call.tool, areq.Statement), "")
		// A prepare-only script has no envelope to report in: the agent wrote the writes itself.
		if so != nil && len(saved)+len(failed)+len(deleted) > 0 {
			return so.agentResult(saved, failed, deleted)
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

// kubeRead is the one kube tool: it checks op, then runs the read through kubeRun.
func (s *Server) kubeRead(ctx context.Context, _ *mcp.CallToolRequest, a kubeReadArgs) (*mcp.CallToolResult, any, error) {
	confirm, msg := confirmText(a.Confirm, kubeNothingRead)
	if msg != "" {
		return errResult(msg)
	}
	call := kubeCall{tool: "kube_read", op: a.Op, clusterID: a.ClusterID, script: a.Script, confirm: confirm}
	switch a.Op {
	case kubeDiscover:
		if a.Script != nil {
			return errResult("discover takes no script")
		}
		return s.kubeDiscoverRead(ctx, call)
	case kubeList:
		call.req = kubeClamp(kubeList, kubeRequest{Kind: a.Kind, Namespace: a.Namespace, LabelSelector: a.LabelSelector, MaxRows: a.MaxRows})
		return s.kubeListRead(ctx, call)
	case kubeDescribe:
		call.req = kubeRequest{Kind: a.Kind, Name: a.Name, Namespace: a.Namespace}
		return s.kubeDescribeRead(ctx, call)
	case kubeLogs:
		call.req = kubeClamp(kubeLogs, kubeRequest{Namespace: a.Namespace, Name: a.Name, Container: a.Container, Previous: a.Previous, Timestamps: a.Timestamps, Tail: a.Tail})
		return s.kubeLogsRead(ctx, call)
	}
	return errResult(fmt.Sprintf("op must be discover, list, describe or logs, not %q", a.Op))
}

// kubeDiscoverResult is discover's answer: the namespaces and the listable kinds of one cluster.
type kubeDiscoverResult struct {
	kubernetes.NamespacesResult
	Kinds   []kubeKindView `json:"kinds"`
	Partial bool           `json:"partial,omitempty"`
}

// kubeDiscoverRead returns namespaces and kinds under one discover approval.
func (s *Server) kubeDiscoverRead(ctx context.Context, call kubeCall) (*mcp.CallToolResult, any, error) {
	return s.kubeRun(ctx, call,
		func(c kubernetes.Cluster, _ kubeRequest) string {
			return kubernetes.NamespacesCommand(c.Context) + "\n" + kubernetes.KindsCommand(c.Context)
		},
		func(ctx context.Context, _ kubeRequest) (any, error) {
			ns, err := s.cfg.Kube.Namespaces(ctx, kubernetes.ClusterArgs{ClusterID: call.clusterID})
			if err != nil {
				return nil, err
			}
			r, err := s.cfg.Kube.Kinds(ctx, kubernetes.ClusterArgs{ClusterID: call.clusterID})
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

func (s *Server) kubeListRead(ctx context.Context, call kubeCall) (*mcp.CallToolResult, any, error) {
	return s.kubeRun(ctx, call,
		func(c kubernetes.Cluster, r kubeRequest) string {
			return kubernetes.ListCommand(c.Context, kubernetes.ListArgs{ClusterID: call.clusterID, Kind: r.Kind, Namespace: r.Namespace, LabelSelector: r.LabelSelector})
		},
		func(ctx context.Context, r kubeRequest) (any, error) {
			res, err := s.cfg.Kube.ListCapped(ctx, kubernetes.ListArgs{
				ClusterID: call.clusterID, Kind: r.Kind, Namespace: r.Namespace, LabelSelector: r.LabelSelector,
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
			out := map[string]any{"columns": cols, "rows": rows, "rowCount": len(rows)}
			if res.Truncated {
				out["truncated"] = true
				out["hint"] = fmt.Sprintf("stopped at maxRows %d: add labelSelector or namespace, or raise maxRows (max %d)", r.MaxRows, kubeListMaxRows)
			}
			return out, nil
		})
}

func (s *Server) kubeDescribeRead(ctx context.Context, call kubeCall) (*mcp.CallToolResult, any, error) {
	return s.kubeRun(ctx, call,
		func(c kubernetes.Cluster, r kubeRequest) string {
			return kubernetes.DescribeCommand(c.Context, kubernetes.DescribeArgs{ClusterID: call.clusterID, Kind: r.Kind, Namespace: r.Namespace, Name: r.Name})
		},
		func(ctx context.Context, r kubeRequest) (any, error) {
			d, err := s.cfg.Kube.Describe(ctx, kubernetes.DescribeArgs{
				ClusterID: call.clusterID, Kind: r.Kind, Namespace: r.Namespace, Name: r.Name,
			})
			if err != nil {
				return nil, err
			}
			res := kubeDescribeResult{Description: d}
			res.Manifest, res.ManifestTruncated = cutLines(d.Manifest, kubeManifestMax)
			if res.ManifestTruncated {
				res.Hint = fmt.Sprintf("manifest cut at %d KiB: read what you need with a transform", kubeManifestMax>>10)
			}
			return res, nil
		})
}

func (s *Server) kubeLogsRead(ctx context.Context, call kubeCall) (*mcp.CallToolResult, any, error) {
	return s.kubeRun(ctx, call,
		func(c kubernetes.Cluster, r kubeRequest) string {
			return kubernetes.LogsCommand(c.Context, kubernetes.LogsReadArgs{Namespace: r.Namespace, Pod: r.Name, Container: r.Container, Previous: r.Previous, Timestamps: r.Timestamps, Tail: r.Tail}, false)
		},
		func(ctx context.Context, r kubeRequest) (any, error) {
			res, err := s.cfg.Kube.ReadLogs(ctx, kubernetes.LogsReadArgs{
				ClusterID: call.clusterID, Namespace: r.Namespace, Pod: r.Name, Container: r.Container,
				Previous: r.Previous, Timestamps: r.Timestamps, Tail: r.Tail,
			})
			if err != nil {
				return nil, err
			}
			out := kubeLogsResult{LogsReadResult: res}
			if res.Truncated {
				out.Hint = "log cut at the size cap: lower tail or filter with a transform"
			}
			return out, nil
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

// addKubeTools registers kube_read next to the database tools.
func (s *Server) addKubeTools(srv *mcp.Server, src llmprompts.Source) {
	sch := llmprompts.RequireNonEmpty(llmprompts.InputSchema[kubeReadArgs](src, llmprompts.DbmcpToolKubeRead.ID(), map[string]any{
		"maxRows": llmprompts.LimitVars{Default: kubeListDefaultRows, Max: kubeListMaxRows},
		"tail":    llmprompts.LimitVars{Default: kubeLogsDefaultTail, Max: kubeLogsMaxTail},
	}), "clusterId", "op")
	sch.Properties["op"].Enum = []any{kubeDiscover, kubeList, kubeDescribe, kubeLogs}
	// A prepare can set any arg, so a read's own args stay optional; kubeRun checks the final request.
	if s.scriptsOn() {
		pinScriptObject(sch)
	} else {
		delete(sch.Properties, "script")
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name: "kube_read", Description: llmprompts.DbmcpToolKubeRead.Text(src, llmprompts.NoVars{}), InputSchema: sch,
	}, withPanicRecovery(s, "kube_read", s.kubeRead))
}
