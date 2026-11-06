package dbmcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
)

// statementTabler is the optional QueryRunner half that names a statement's tables (*adapterhost.Router).
type statementTabler interface {
	StatementTables(ctx context.Context, connectionID, path, statement string) (adapterhost.TablesResult, error)
}

const (
	unlistedMaxShown = 10
	tablesViewMax    = 200
)

// allowListOf parses summary.McpTables. A nil set means all tables. A stored entry that fails to parse
// returns an error: callers refuse, never fall back to all.
func allowListOf(summary model.ConnectionSummary) (*tableref.Set, error) {
	if summary.McpTables == nil {
		return nil, nil
	}
	return tableref.NewSet(summary.Kind, summary.McpTables)
}

func badListResult(summary model.ConnectionSummary) (*mcp.CallToolResult, any, error) {
	return errResult(fmt.Sprintf("connection %q has a table allow list this app cannot read; fix it in the connection's MCP tab", summary.Name))
}

func limitedPrefix(summary model.ConnectionSummary) string {
	return fmt.Sprintf("connection %q limits MCP to listed tables", summary.Name)
}

// checkStatementTables refuses a statement under a non-all list when it touches an unlisted table, cannot be
// parsed, or uses a construct the list cannot check. A nil res and err means pass. The list is not a P528
// limit: no exemption applies.
func (s *Server) checkStatementTables(ctx context.Context, summary model.ConnectionSummary, path, sql string) (*mcp.CallToolResult, any, error) {
	set, err := allowListOf(summary)
	if err != nil {
		return badListResult(summary)
	}
	if set == nil {
		return nil, nil, nil
	}
	tabler, ok := s.cfg.Query.(statementTabler)
	if !ok {
		return errResult(limitedPrefix(summary) + ", and this server cannot check statements against them")
	}
	tr, err := tabler.StatementTables(ctx, summary.ID, path, sql)
	if err != nil {
		if s.maskedConnection(summary.ID) {
			return maskedToolError(err)
		}
		return toolError(err)
	}
	if tr.Refusal != "" {
		return errResult(tablesRefusal(summary, tr.Refusal))
	}
	var unlisted []string
	seen := map[string]bool{}
	for _, r := range tr.Refs {
		if set.Allows(r) {
			continue
		}
		name := tableref.Format(summary.Kind, r)
		if !seen[name] {
			seen[name] = true
			unlisted = append(unlisted, name)
		}
	}
	if len(unlisted) == 0 {
		return nil, nil, nil
	}
	sort.Strings(unlisted)
	shown := unlisted
	more := ""
	if len(shown) > unlistedMaxShown {
		more = fmt.Sprintf(" and %d more", len(shown)-unlistedMaxShown)
		shown = shown[:unlistedMaxShown]
	}
	return errResult(fmt.Sprintf("%s; not listed: %s%s. list_connections shows the tables field.", limitedPrefix(summary), strings.Join(shown, ", "), more))
}

func tablesRefusal(summary model.ConnectionSummary, refusal string) string {
	if msg, ok := strings.CutPrefix(refusal, "parse error: "); ok {
		text := fmt.Sprintf("%s, and this statement could not be parsed to check them: %s. Rewrite it in plain statements, or ask the user to allow all tables.", limitedPrefix(summary), msg)
		if summary.Kind == "mariadb" {
			text += " MariaDB-only syntax cannot be checked."
		}
		return text
	}
	return fmt.Sprintf("%s; refused: %s (it can reach tables the list cannot check). Use plain statements on listed tables.", limitedPrefix(summary), refusal)
}

// allowedPath reports whether a tree path may be read under set. A path at or below a relation needs that
// relation listed; requireRelation also refuses a path that names no relation. Empty means allowed.
func allowedPath(summary model.ConnectionSummary, set *tableref.Set, path string, requireRelation bool) string {
	if set == nil {
		return ""
	}
	p, err := model.DecodePath(summary.ID, path)
	if err != nil {
		return fmt.Sprintf("%s, and this path could not be read", limitedPrefix(summary))
	}
	ref, ok := tableref.FromPath(summary.Kind, p.Segments)
	if !ok {
		if requireRelation {
			return fmt.Sprintf("%s; this path is not a listed table", limitedPrefix(summary))
		}
		return ""
	}
	if !set.Allows(ref) {
		return fmt.Sprintf("%s; not listed: %s. list_connections shows the tables field.", limitedPrefix(summary), tableref.Format(summary.Kind, ref))
	}
	return ""
}

// filterNodes drops relation nodes the list does not allow. A relation node whose path maps to no relation is
// dropped too.
func filterNodes(summary model.ConnectionSummary, set *tableref.Set, nodes []model.TreeNode) []model.TreeNode {
	if set == nil {
		return nodes
	}
	out := make([]model.TreeNode, 0, len(nodes))
	for _, n := range nodes {
		if tableref.IsRelationKind(n.Kind) {
			p, err := model.DecodePath(summary.ID, n.Path)
			if err != nil {
				continue
			}
			ref, ok := tableref.FromPath(summary.Kind, p.Segments)
			if !ok || !set.Allows(ref) {
				continue
			}
		}
		out = append(out, n)
	}
	return out
}

// filterRelations drops relations of the container at path that the list does not allow. A container path of
// the wrong shape drops all.
func filterRelations(summary model.ConnectionSummary, set *tableref.Set, path string, rels []model.RelationColumns) []model.RelationColumns {
	if set == nil {
		return rels
	}
	p, err := model.DecodePath(summary.ID, path)
	if err != nil {
		return []model.RelationColumns{}
	}
	out := make([]model.RelationColumns, 0, len(rels))
	for _, r := range rels {
		segs := append(append([]model.PathSegment{}, p.Segments...), model.PathSegment{Kind: r.Kind, Name: r.Name})
		ref, ok := tableref.FromPath(summary.Kind, segs)
		if ok && len(segs) == containerDepth(summary.Kind)+1 && set.Allows(ref) {
			out = append(out, r)
		}
	}
	return out
}

// containerDepth is how many path segments the container of a relation has.
func containerDepth(kind string) int {
	if kind == "postgres" {
		return 2
	}
	return 1
}

// allowedTablesView is list_connections' tables field.
type allowedTablesView struct {
	All   bool     `json:"all"`
	Names []string `json:"names"`
	More  int      `json:"more,omitempty"`
}

func tablesView(summary model.ConnectionSummary) *allowedTablesView {
	set, err := allowListOf(summary)
	if set == nil && err == nil {
		return &allowedTablesView{All: true, Names: []string{}}
	}
	if err != nil {
		return &allowedTablesView{Names: []string{}}
	}
	names := set.Entries()
	v := &allowedTablesView{Names: names}
	if len(names) > tablesViewMax {
		v.Names = names[:tablesViewMax]
		v.More = len(names) - tablesViewMax
	}
	if v.Names == nil {
		v.Names = []string{}
	}
	return v
}

// filterMaskRules keeps the rules that do not name an unlisted table: a rule's table name is itself a table
// name list_connections would otherwise show.
func filterMaskRules(set *tableref.Set, rules []model.MaskRule) []model.MaskRule {
	if set == nil {
		return rules
	}
	out := make([]model.MaskRule, 0, len(rules))
	for _, r := range rules {
		if r.TableName == "*" || set.AllowsName(r.TableName) {
			out = append(out, r)
		}
	}
	return out
}
