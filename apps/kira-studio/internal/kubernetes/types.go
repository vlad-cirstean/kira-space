package kubernetes

// Config states.
const (
	ConfigOK      = "ok"
	ConfigMissing = "missing"
	ConfigInvalid = "invalid"
)

// Status states and reasons.
const (
	StatusOK          = "ok"
	StatusUnavailable = "unavailable"

	ReasonUnreachable  = "unreachable"
	ReasonUnauthorized = "unauthorized"
	ReasonForbidden    = "forbidden"
	ReasonPlugin       = "plugin"
	ReasonConfig       = "config"
	ReasonTimeout      = "timeout"
)

// ConfigInfo reports where the kubeconfig came from and whether it loaded.
type ConfigInfo struct {
	State   string   `json:"state"`
	Paths   []string `json:"paths"`
	Message string   `json:"message"`
}

// Cluster is one kubeconfig context with its stable stored id.
type Cluster struct {
	ID        string      `json:"id"`
	Context   string      `json:"context"`
	Cluster   string      `json:"cluster"`
	Server    string      `json:"server"`
	User      string      `json:"user"`
	Namespace string      `json:"namespace"`
	Current   bool        `json:"current"`
	Source    string      `json:"source"`
	Mcp       McpSettings `json:"mcp"`
}

// McpSettings is a cluster's MCP exposure: a description for the AI and one mode per operation group
// (deny, prompt or allow).
type McpSettings struct {
	Description string `json:"description"`
	Discover    string `json:"discover"`
	List        string `json:"list"`
	Describe    string `json:"describe"`
	Logs        string `json:"logs"`
}

// ReorderArgs is Reorder's argument: cluster ids in the wanted order.
type ReorderArgs struct {
	IDs []string `json:"ids"`
}

// SetMcpArgs is SetMcp's argument.
type SetMcpArgs struct {
	ClusterID string      `json:"clusterId"`
	Mcp       McpSettings `json:"mcp"`
}

// ClustersResult is Clusters' result.
type ClustersResult struct {
	Config   ConfigInfo `json:"config"`
	Clusters []Cluster  `json:"clusters"`
}

// Status is a cluster probe result. Trouble is a value, not an error.
type Status struct {
	State         string `json:"state"`
	Reason        string `json:"reason"`
	Message       string `json:"message"`
	ServerVersion string `json:"serverVersion"`
}

// NamespacesResult lists namespaces; Restricted means the list call was forbidden.
type NamespacesResult struct {
	Namespaces []string `json:"namespaces"`
	Restricted bool     `json:"restricted"`
	Fallback   string   `json:"fallback"`
}

// Kind is one listable resource type.
type Kind struct {
	Key        string `json:"key"`
	Group      string `json:"group"`
	Version    string `json:"version"`
	Resource   string `json:"resource"`
	Kind       string `json:"kind"`
	Namespaced bool   `json:"namespaced"`
	Category   string `json:"category"`
	Logs       bool   `json:"logs"`
}

// KindsResult lists kinds; Partial means some API groups failed discovery.
type KindsResult struct {
	Kinds   []Kind `json:"kinds"`
	Partial bool   `json:"partial"`
}

// Column is one list column.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Row is one listed object; Cells follow Columns.
type Row struct {
	Key       string   `json:"key"`
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Cells     []string `json:"cells"`
}

// ListResult is a kubectl-style table.
type ListResult struct {
	Columns   []Column `json:"columns"`
	Rows      []Row    `json:"rows"`
	Truncated bool     `json:"truncated"`
}

// Owner is an owner reference.
type Owner struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// Condition is one status condition.
type Condition struct {
	Type           string `json:"type"`
	Status         string `json:"status"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	LastTransition string `json:"lastTransition"`
}

// ContainerRef names one pod container.
type ContainerRef struct {
	Name string `json:"name"`
	Init bool   `json:"init"`
}

// Event is one involved-object event.
type Event struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Count   int32  `json:"count"`
	First   string `json:"first"`
	Last    string `json:"last"`
	Source  string `json:"source"`
}

// Description is the describe view of one object. Secret values never appear in it.
type Description struct {
	Kind        string            `json:"kind"`
	APIVersion  string            `json:"apiVersion"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	UID         string            `json:"uid"`
	Created     string            `json:"created"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	Owners      []Owner           `json:"owners"`
	Conditions  []Condition       `json:"conditions"`
	Containers  []ContainerRef    `json:"containers"`
	Events      []Event           `json:"events"`
	Manifest    string            `json:"manifest"`
}

// Wire argument shapes.

type StatusArgs struct {
	ClusterID string `json:"clusterId"`
	Refresh   bool   `json:"refresh"`
	// Background marks a refetch of loaded data: it skips the op log.
	Background bool `json:"background,omitempty"`
}

type ClusterArgs struct {
	ClusterID  string `json:"clusterId"`
	Background bool   `json:"background,omitempty"`
}

type ListArgs struct {
	ClusterID     string `json:"clusterId"`
	Kind          string `json:"kind"`
	Namespace     string `json:"namespace"`
	LabelSelector string `json:"labelSelector"`
	Background    bool   `json:"background,omitempty"`
}

type DescribeArgs struct {
	ClusterID  string `json:"clusterId"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Background bool   `json:"background,omitempty"`
}

type StreamArgs struct {
	StreamID string `json:"streamId"`
}
