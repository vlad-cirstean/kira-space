package dbmcp

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/queryplan"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The sections of list, in the order the result shows them.
const (
	listConnections  = "connections"
	listClusters     = "clusters"
	listEnvironments = "environments"
	listScripts      = "scripts"
	listSaved        = "saved"
)

type listArgs struct {
	What []string `json:"what,omitempty"`
}

// listSections are the sections this server can show now: each one's feature is on.
func (s *Server) listSections() []string {
	out := []string{listConnections}
	if s.cfg.Kube != nil {
		out = append(out, listClusters)
	}
	if s.apiEnabled() {
		out = append(out, listEnvironments)
	}
	if s.cfg.Scripts != nil {
		out = append(out, listScripts)
	}
	if s.cfg.Kept != nil {
		out = append(out, listSaved)
	}
	return out
}

// listedConnection is one connection as the agent sees it: only what changes a decision, empty fields left out.
type listedConnection struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status"`
	ReadOnly    bool     `json:"readOnly,omitempty"`
	Access      string   `json:"access"`
	Review      []string `json:"review,omitempty"`
	Enforcement string   `json:"enforcement"`
	// TimeoutSeconds is the statement timeout; omitted while off.
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
	// ExpensiveRows is the row estimate over which a statement is planned first; omitted while the check is off.
	ExpensiveRows int `json:"expensiveRows,omitempty"`
	// Tables is the only tables MCP may touch; nil means all. An empty list means none.
	Tables          []string `json:"tables,omitzero"`
	TablesMore      int      `json:"tablesMore,omitempty"`
	MaskedColumns   []string `json:"maskedColumns,omitempty"`
	WithheldColumns []string `json:"withheldColumns,omitempty"`
	WithheldMore    int      `json:"withheldMore,omitempty"`
	Can             []string `json:"can"`
	ParamSyntax     string   `json:"paramSyntax,omitempty"`
	QueryFormat     string   `json:"queryFormat,omitempty"`
}

// paramSyntaxOf is how a kind's bind placeholders read; the agent finds it in list, not in the tool schema.
func paramSyntaxOf(kind string) string {
	const values = " A value is text, number, boolean or a flat list of one type."
	switch kind {
	case "postgres":
		return "$1, $2 in param order; a list binds as an array (id = ANY($1))." + values
	case "mysql", "mariadb":
		return "one ? per entry, in order; a list binds as JSON text (id IN (SELECT j.v FROM JSON_TABLE(?, '$[*]' COLUMNS (v BIGINT PATH '$')) j); text items: JSON_CONTAINS(?, JSON_QUOTE(col)))." + values
	case "sqlite":
		return "? in order; a list binds as JSON text (id IN (SELECT value FROM json_each(?)))." + values
	case "clickhouse":
		return "{name:Type} with the saved name, once each (id IN {ids:Array(UInt64)})." + values
	}
	return ""
}

// kibanaQueryFormat is run_query's sql on kibana.
const kibanaQueryFormat = `sql is a JSON object: dataView (id from describe), query, language (kuery or lucene), timeRange ({"from","to"} ISO-8601 or date math, or null), filters ([{"field","operator","values","disabled"}]; operator is, is_not, is_one_of, is_not_one_of, exists, does_not_exist or between), sort ([{"field","order"}]), size, timeZone (IANA).`

func (s *Server) listedConnectionOf(c model.ConnectionSummary) listedConnection {
	state := s.cfg.Conns.StateOf(c.ID)
	access := accessOf(c)
	v := listedConnection{
		ID: c.ID, Name: c.Name, Kind: c.Kind, Description: c.McpDescription, Status: state.Status,
		ReadOnly: c.ReadOnly, Access: access.Summary, Enforcement: enforcementOf(c.Kind),
		TimeoutSeconds: c.McpStatementTimeoutSec, Can: []string{},
	}
	rv := s.reviewViewOf(c)
	for _, m := range []struct {
		held bool
		name string
	}{{rv.Read, modeRead}, {rv.Write, modeWrite}, {rv.DDL, modeDDL}, {rv.Masked, "browse"}} {
		if m.held {
			v.Review = append(v.Review, m.name)
		}
	}
	if ExpensiveCheckOf(c) == expensiveCheckPlan {
		v.ExpensiveRows = s.thresholdFor(c)
	}
	if t := tablesView(c); !t.All {
		v.Tables, v.TablesMore = t.Names, t.More
	}
	if set, err := allowListOf(c); err == nil {
		if rules, err := s.cfg.MaskRules.List(c.ID); err == nil {
			v.MaskedColumns = maskedColumnsFor(filterMaskRules(set, rules))
		}
	}
	if cr := s.columnReviewViewOf(c); cr != nil {
		v.WithheldColumns, v.WithheldMore = cr.Withheld, cr.More
	}
	if access.Masked.Allowed {
		v.Can = append(v.Can, "browse")
	}
	if queryplan.Supported(c.Kind) {
		v.Can = append(v.Can, "explain")
	}
	if _, _, ok := adapters.ParamDialectOf(c.Kind); ok {
		v.Can = append(v.Can, "params")
		v.ParamSyntax = paramSyntaxOf(c.Kind)
	}
	if s.scriptsOn() {
		v.Can = append(v.Can, "scripts")
	}
	if c.Kind == "kibana" {
		v.QueryFormat = kibanaQueryFormat
	}
	return v
}

// connectionsListing is every exposed connection. A connection the user has not exposed is absent, not
// listed and denied: its existence is not the agent's business. It is not gated on read mode: its job is to
// show a connection that denies read but allows browse, write or DDL.
func (s *Server) connectionsListing() ([]listedConnection, error) {
	conns, err := s.cfg.Conns.List()
	if err != nil {
		return nil, err
	}
	out := []listedConnection{}
	for _, c := range conns {
		if c.McpEnabled {
			out = append(out, s.listedConnectionOf(c))
		}
	}
	return out, nil
}

type listedCluster struct {
	ID          string `json:"id"`
	Context     string `json:"context"`
	Namespace   string `json:"namespace,omitempty"`
	Description string `json:"description,omitempty"`
	Approval    string `json:"approval"`
	Review      bool   `json:"review,omitempty"`
}

func (s *Server) clustersListing(ctx context.Context) ([]listedCluster, error) {
	res, err := s.cfg.Kube.Clusters(ctx)
	if err != nil {
		return nil, err
	}
	out := []listedCluster{}
	for _, c := range res.Clusters {
		if kubeExposed(c.Mcp) {
			out = append(out, listedCluster{
				ID: c.ID, Context: c.Context, Namespace: c.Namespace, Description: c.Mcp.Description,
				Approval: c.Mcp.Approval, Review: s.kubeReviewFor(c),
			})
		}
	}
	return out, nil
}

type listedEnvironment struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	AutoReads   bool               `json:"autoReads,omitempty"`
	Review      []string           `json:"review,omitempty"`
	Variables   []agentapi.VarView `json:"variables"`
}

func (s *Server) environmentsListing() ([]listedEnvironment, error) {
	envs, err := s.cfg.API.Environments()
	if err != nil {
		return nil, fmt.Errorf("the environments could not be read")
	}
	out := []listedEnvironment{}
	for _, e := range envs {
		info := agentapi.EnvInfo{Review: e.Review}
		le := listedEnvironment{ID: e.ID, Name: e.Name, Description: e.Description, AutoReads: e.AutoApproveReads, Variables: e.Variables}
		if s.apiReviewSetting(info, true) {
			le.Review = append(le.Review, "read")
		}
		if s.apiReviewSetting(info, false) {
			le.Review = append(le.Review, "write")
		}
		out = append(out, le)
	}
	return out, nil
}

type listedScriptParam struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Options  []string `json:"options,omitempty"`
	Default  []string `json:"default,omitempty"`
	Required bool     `json:"required,omitempty"`
}

type listedScript struct {
	ScriptID    string              `json:"scriptId"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Approval    string              `json:"approval"`
	Mode        string              `json:"mode"`
	Params      []listedScriptParam `json:"params,omitempty"`
}

// scriptsListing is the exposed scripts. A secret parameter is left out: the agent cannot set it.
func (s *Server) scriptsListing() ([]listedScript, error) {
	list, err := s.cfg.Scripts.Exposed()
	if err != nil {
		return nil, err
	}
	out := []listedScript{}
	for _, e := range list {
		v := listedScript{ScriptID: e.ID, Name: e.Name, Description: e.Description, Approval: e.Run, Mode: e.mode()}
		for _, p := range e.Params {
			if !p.Secret {
				v.Params = append(v.Params, listedScriptParam{Name: p.Name, Type: p.Type, Options: p.Options, Default: p.Default, Required: p.Required})
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// savedListing names what the caller can use. Never a value, a preview or the request that made it.
func (s *Server) savedListing(ctx context.Context) []keptListEntry {
	list := s.cfg.Kept.List(keptScope(ctx))
	out := make([]keptListEntry, 0, len(list))
	for _, m := range list {
		out = append(out, keptListEntry{
			Name: m.Name, Type: m.Type, Items: m.Items, Secret: m.Secret,
			Source:           keptListSource{Kind: string(m.Source.Kind), Tool: m.Source.Tool},
			ExpiresInSeconds: max(int(math.Ceil(time.Until(m.ExpiresAt).Seconds())), 0),
		})
	}
	return out
}

// listResult holds the sections asked for; a section that is off or not asked for is absent, an empty one is [].
type listResult struct {
	Connections  []listedConnection  `json:"connections,omitzero"`
	Clusters     []listedCluster     `json:"clusters,omitzero"`
	Environments []listedEnvironment `json:"environments,omitzero"`
	Scripts      []listedScript      `json:"scripts,omitzero"`
	Saved        []keptListEntry     `json:"saved,omitzero"`
}

func (s *Server) list(ctx context.Context, _ *mcp.CallToolRequest, a listArgs) (*mcp.CallToolResult, any, error) {
	avail := s.listSections()
	want := avail
	if len(a.What) > 0 {
		want = nil
		for _, w := range a.What {
			if !slices.Contains(avail, w) {
				return errResult(fmt.Sprintf("what %q is not available; use one of: %v", w, avail))
			}
			want = append(want, w)
		}
	}
	var out listResult
	var err error
	for _, w := range avail {
		if !slices.Contains(want, w) {
			continue
		}
		switch w {
		case listConnections:
			out.Connections, err = s.connectionsListing()
		case listClusters:
			out.Clusters, err = s.clustersListing(ctx)
		case listEnvironments:
			out.Environments, err = s.environmentsListing()
		case listScripts:
			out.Scripts, err = s.scriptsListing()
		case listSaved:
			out.Saved = s.savedListing(ctx)
		}
		if err != nil {
			return toolError(err)
		}
	}
	return jsonResult(out)
}

// listSchema is list's input schema: what is limited to the sections that are on.
func listSchema(src llmprompts.Source, sections []string) *jsonschema.Schema {
	sch := llmprompts.InputSchema[listArgs](src, llmprompts.DbmcpToolList.ID(), nil)
	enum := make([]any, len(sections))
	for i, n := range sections {
		enum[i] = n
	}
	sch.Properties["what"].Items = &jsonschema.Schema{Type: "string", Enum: enum}
	return sch
}

func (s *Server) addListTool(srv *mcp.Server, src llmprompts.Source) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list",
		Description: llmprompts.DbmcpToolList.Text(src, llmprompts.NoVars{}),
		InputSchema: listSchema(src, s.listSections()),
	}, withPanicRecovery(s, "list", s.list))
}
