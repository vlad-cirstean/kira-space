package dbmcp

import (
	"context"
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

// Permission groups, the stored words of kube_clusters.mcp_*_mode.
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
	ClusterID     string `json:"clusterId"`
	Kind          string `json:"kind"`
	Namespace     string `json:"namespace,omitempty"`
	LabelSelector string `json:"labelSelector,omitempty"`
	MaxRows       int    `json:"maxRows,omitempty"`
}

type kubeDescribeArgs struct {
	ClusterID string `json:"clusterId"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type kubePodLogsArgs struct {
	ClusterID  string `json:"clusterId"`
	Namespace  string `json:"namespace"`
	Pod        string `json:"pod"`
	Container  string `json:"container,omitempty"`
	Previous   bool   `json:"previous,omitempty"`
	Timestamps bool   `json:"timestamps,omitempty"`
	Tail       int    `json:"tail,omitempty"`
}

type kubePermissions struct {
	Discover string `json:"discover"`
	List     string `json:"list"`
	Describe string `json:"describe"`
	Logs     string `json:"logs"`
}

type kubeClusterView struct {
	ID          string          `json:"id"`
	Context     string          `json:"context"`
	Cluster     string          `json:"cluster"`
	Server      string          `json:"server"`
	Namespace   string          `json:"namespace"`
	Current     bool            `json:"current"`
	Description string          `json:"description"`
	Permissions kubePermissions `json:"permissions"`
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

func modeOfGroup(m kubernetes.McpSettings, group string) string {
	switch group {
	case kubeDiscover:
		return m.Discover
	case kubeList:
		return m.List
	case kubeDescribe:
		return m.Describe
	default:
		return m.Logs
	}
}

// kubeExposed reports whether any operation of the cluster is not denied. A fully denied cluster
// is hidden, like an unexposed connection.
func kubeExposed(m kubernetes.McpSettings) bool {
	for _, g := range []string{kubeDiscover, kubeList, kubeDescribe, kubeLogs} {
		if modeOfGroup(m, g) != "deny" {
			return true
		}
	}
	return false
}

// resolveCluster finds an exposed cluster by id; found is false for an unknown or fully denied one.
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

// kubeRun is the gate every cluster read passes: exposure, the group's mode, the approval for
// prompt, then the read. The kubernetes service records the op-log row.
func (s *Server) kubeRun(ctx context.Context, clusterID, group, namespace string, statement func(kubernetes.Cluster) string, read func(context.Context) (any, error)) (*mcp.CallToolResult, any, error) {
	c, found, err := s.resolveCluster(ctx, clusterID)
	if err != nil {
		return toolError(err)
	}
	if !found {
		return noClusterResult(clusterID)
	}
	stmt := statement(c)
	switch modeOfGroup(c.Mcp, group) {
	case "allow":
	case "prompt":
		outcome := s.cfg.Approvals.Request(ctx, ApprovalRequest{
			ConnectionID: c.ID, ConnectionName: c.Context, Kind: "kubernetes",
			Class: adapters.ClassRead, Statement: stmt, Reason: ApprovalReasonPermission,
			Cluster: &ApprovalCluster{Context: c.Context, Server: c.Server, Namespace: namespace, Operation: group},
			Origin:  originFrom(ctx),
		})
		if outcome != ApprovalApproved {
			return approvalOutcomeError(outcome, fmt.Sprintf("Kubernetes read on %q", c.Context))
		}
		// The wait can last up to ApprovalTimeout: a tightened mode or a hidden cluster wins.
		c, found, err = s.resolveCluster(ctx, clusterID)
		if err != nil {
			return toolError(err)
		}
		if !found {
			return noClusterResult(clusterID)
		}
		if modeOfGroup(c.Mcp, group) == "deny" {
			return errResult(kubeDeniedText(c.Context, group, "now deny"))
		}
	default:
		return errResult(kubeDeniedText(c.Context, group, "deny"))
	}

	out, err := read(ctx)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(out)
}

func kubeDeniedText(name, group, verb string) string {
	return fmt.Sprintf("cluster %q's MCP permissions %s the %s operation; change them in the cluster's MCP settings", name, verb, group)
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
			Permissions: kubePermissions{Discover: c.Mcp.Discover, List: c.Mcp.List, Describe: c.Mcp.Describe, Logs: c.Mcp.Logs},
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
	return s.kubeRun(ctx, a.ClusterID, kubeDiscover, "",
		func(c kubernetes.Cluster) string {
			return kubernetes.NamespacesCommand(c.Context) + "\n" + kubernetes.KindsCommand(c.Context)
		},
		func(ctx context.Context) (any, error) {
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
	maxRows := clampOr(a.MaxRows, kubeListDefaultRows, kubeListMaxRows)
	return s.kubeRun(ctx, a.ClusterID, kubeList, a.Namespace,
		func(c kubernetes.Cluster) string {
			return kubernetes.ListCommand(c.Context, kubernetes.ListArgs{ClusterID: a.ClusterID, Kind: a.Kind, Namespace: a.Namespace, LabelSelector: a.LabelSelector})
		},
		func(ctx context.Context) (any, error) {
			r, err := s.cfg.Kube.ListCapped(ctx, kubernetes.ListArgs{
				ClusterID: a.ClusterID, Kind: a.Kind, Namespace: a.Namespace, LabelSelector: a.LabelSelector,
			}, maxRows)
			if err != nil {
				return nil, err
			}
			cols := make([]string, 0, len(r.Columns))
			for _, col := range r.Columns {
				cols = append(cols, col.Name)
			}
			rows := make([]kubeRowView, 0, len(r.Rows))
			for _, row := range r.Rows {
				rows = append(rows, kubeRowView{Name: row.Name, Namespace: row.Namespace, Cells: row.Cells})
			}
			return map[string]any{"columns": cols, "rows": rows, "rowCount": len(rows), "truncated": r.Truncated}, nil
		})
}

func (s *Server) kubeDescribeTool(ctx context.Context, _ *mcp.CallToolRequest, a kubeDescribeArgs) (*mcp.CallToolResult, any, error) {
	return s.kubeRun(ctx, a.ClusterID, kubeDescribe, a.Namespace,
		func(c kubernetes.Cluster) string {
			return kubernetes.DescribeCommand(c.Context, kubernetes.DescribeArgs{ClusterID: a.ClusterID, Kind: a.Kind, Namespace: a.Namespace, Name: a.Name})
		},
		func(ctx context.Context) (any, error) {
			d, err := s.cfg.Kube.Describe(ctx, kubernetes.DescribeArgs{
				ClusterID: a.ClusterID, Kind: a.Kind, Namespace: a.Namespace, Name: a.Name,
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
	tail := clampOr(a.Tail, kubeLogsDefaultTail, kubeLogsMaxTail)
	return s.kubeRun(ctx, a.ClusterID, kubeLogs, a.Namespace,
		func(c kubernetes.Cluster) string {
			return kubernetes.LogsCommand(c.Context, kubernetes.LogsReadArgs{Namespace: a.Namespace, Pod: a.Pod, Container: a.Container, Previous: a.Previous, Timestamps: a.Timestamps, Tail: tail}, false)
		},
		func(ctx context.Context) (any, error) {
			return s.cfg.Kube.ReadLogs(ctx, kubernetes.LogsReadArgs{
				ClusterID: a.ClusterID, Namespace: a.Namespace, Pod: a.Pod, Container: a.Container,
				Previous: a.Previous, Timestamps: a.Timestamps, Tail: tail,
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
	mcp.AddTool(srv, &mcp.Tool{
		Name: "kube_list_clusters", Description: desc(llmprompts.DbmcpToolKubeListClusters),
	}, withPanicRecovery(s, "kube_list_clusters", s.kubeListClusters))
	mcp.AddTool(srv, &mcp.Tool{
		Name: "kube_discover", Description: desc(llmprompts.DbmcpToolKubeDiscover),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[kubeClusterArgs](src, llmprompts.DbmcpToolKubeDiscover.ID(), nil), "clusterId"),
	}, withPanicRecovery(s, "kube_discover", s.kubeDiscoverTool))
	mcp.AddTool(srv, &mcp.Tool{
		Name: "kube_list_resources", Description: desc(llmprompts.DbmcpToolKubeListResources),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[kubeListArgs](src, llmprompts.DbmcpToolKubeListResources.ID(),
			map[string]any{"maxRows": llmprompts.LimitVars{Default: kubeListDefaultRows, Max: kubeListMaxRows}}), "clusterId", "kind"),
	}, withPanicRecovery(s, "kube_list_resources", s.kubeListResources))
	mcp.AddTool(srv, &mcp.Tool{
		Name: "kube_describe", Description: desc(llmprompts.DbmcpToolKubeDescribe),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[kubeDescribeArgs](src, llmprompts.DbmcpToolKubeDescribe.ID(), nil), "clusterId", "kind", "name"),
	}, withPanicRecovery(s, "kube_describe", s.kubeDescribeTool))
	mcp.AddTool(srv, &mcp.Tool{
		Name: "kube_pod_logs", Description: desc(llmprompts.DbmcpToolKubePodLogs),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[kubePodLogsArgs](src, llmprompts.DbmcpToolKubePodLogs.ID(),
			map[string]any{"tail": llmprompts.LimitVars{Default: kubeLogsDefaultTail, Max: kubeLogsMaxTail}}), "clusterId", "namespace", "pod"),
	}, withPanicRecovery(s, "kube_pod_logs", s.kubePodLogs))
}
