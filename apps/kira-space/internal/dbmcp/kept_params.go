package dbmcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Limits on run_query's params.
const (
	maxParamEntries  = 64
	maxParamDistinct = 16
	// maxClickHouseParamBytes mirrors the adapter's cap: values ride the URL.
	maxClickHouseParamBytes = 256 << 10
)

// keptParams is what resolveParams found: the names as the agent listed them, the kept entries as they stood at
// resolve time (IDs, never values) and how each name shows on the card.
type keptParams struct {
	names []string
	// uses holds one entry per distinct name, in first-use order, with its statement and param sites.
	uses []agentapi.KeptUse
	// binds is one per name entry, Label and Name set; bind adds the values.
	binds []model.BindParam
	// placeholders[i] is how names[i] shows on the card: `$1`, `? 1` or `{ids:Array(UInt64)}`.
	placeholders []string
}

func (kp *keptParams) usesOf() []agentapi.KeptUse {
	if kp == nil {
		return nil
	}
	return kp.uses
}

func (kp *keptParams) namesOf() []string {
	if kp == nil {
		return nil
	}
	return kp.names
}

func (kp *keptParams) placeholdersOf() []string {
	if kp == nil {
		return nil
	}
	return kp.placeholders
}

// keptChangedError is a kept variable that is not the one the statement was checked against.
type keptChangedError struct {
	name string
	gone bool
}

func (e *keptChangedError) Error() string {
	if e.gone {
		return fmt.Sprintf("no kept variable %q (it may have expired or been cleared); nothing ran. Call list_kept_variables, produce it again, then repeat the query.", e.name)
	}
	return fmt.Sprintf("kept variable %q changed after the statement was checked; nothing ran. Produce it again, then repeat the query.", e.name)
}

func unknownKeptText(name string) string {
	return fmt.Sprintf("no kept variable %q (it may have expired or been cleared); call list_kept_variables", name)
}

// resolveParams checks params against the statement and snapshots the kept variables they name. It runs
// after resolveVerdict: the class, table allow list and column review were decided on the statement text, which
// placeholders do not change. A call without params passes through untouched.
func (s *Server) resolveParams(ctx context.Context, args runQueryArgs, rv *queryResolution) (res *mcp.CallToolResult, out any, err error, done bool) {
	if len(args.Params) == 0 {
		return nil, nil, nil, false
	}
	refuse := func(format string, a ...any) (*mcp.CallToolResult, any, error, bool) {
		res, out, err := errResult(fmt.Sprintf(format, a...))
		return res, out, err, true
	}
	style, dialect, ok := adapters.ParamDialectOf(rv.summary.Kind)
	if !ok {
		return refuse("connection %q (%s) takes no params; kept variables bind into SQL statements only (postgres, mysql, mariadb, sqlite, clickhouse)", rv.summary.Name, rv.summary.Kind)
	}
	if len(args.Params) > maxParamEntries {
		return refuse("params lists %d names; the most is %d", len(args.Params), maxParamEntries)
	}
	distinct := distinctNames(args.Params)
	if len(distinct) > maxParamDistinct {
		return refuse("params uses %d different kept variables; the most is %d", len(distinct), maxParamDistinct)
	}
	if adapters.SecondStatement(args.SQL, dialect) {
		return refuse("params need exactly one statement; this one has a second after a ;")
	}
	sites, serr := adapters.PlaceholderSites(args.SQL, dialect, style)
	if serr != nil {
		return refuse("%s", serr.Error())
	}
	positions, msg := matchPlaceholders(args.SQL, args.Params, sites, style)
	if msg != "" {
		return refuse("%s", msg)
	}

	if s.cfg.Kept == nil {
		return refuse("%s", unknownKeptText(args.Params[0]))
	}
	scope := keptScope(ctx)
	entries := make(map[string]keptvars.Entry, len(distinct))
	total := 0
	for _, name := range distinct {
		e, ok := keptvars.Entry{}, false
		if keptvars.ValidName(name) == nil {
			e, ok = s.cfg.Kept.Get(scope, name)
		}
		if !ok {
			return refuse("%s", unknownKeptText(name))
		}
		if e.Secret {
			return refuse("kept variable %q is secret; secrets bind into api_request only, never into SQL", name)
		}
		if _, derr := adapters.DecodeParam(name, e.Value); derr != nil {
			return refuse("%s", derr.Error())
		}
		entries[name] = e
	}
	if style == adapters.ParamBraces {
		for _, name := range args.Params {
			total += len(name) + entries[name].Bytes
		}
		if total > maxClickHouseParamBytes {
			return refuse("the bound values are over the %d KiB ClickHouse takes in one request; bind fewer or smaller values", maxClickHouseParamBytes>>10)
		}
	}

	kp := &keptParams{names: slices.Clone(args.Params), placeholders: make([]string, len(args.Params))}
	siteOf := map[string][]agentapi.KeptSite{}
	for i, name := range args.Params {
		kp.binds = append(kp.binds, model.BindParam{Label: "{{kept." + name + "}}", Name: braceName(style, name)})
		kp.placeholders[i] = positions[i].display
		for _, ps := range positions[i].sites {
			siteOf[name] = append(siteOf[name], agentapi.KeptSite{
				Field: "statement", From: keptvars.ToUTF16(args.SQL, ps.From), To: keptvars.ToUTF16(args.SQL, ps.To),
			})
		}
		siteOf[name] = append(siteOf[name], agentapi.KeptSite{Field: "param", Index: i})
	}
	for _, name := range distinct {
		kp.uses = append(kp.uses, agentapi.NewKeptUse(entries[name], siteOf[name]))
	}
	rv.params = kp
	return nil, nil, nil, false
}

func braceName(style adapters.ParamStyle, name string) string {
	if style == adapters.ParamBraces {
		return name
	}
	return ""
}

// distinctNames lists each name once, in first-use order.
func distinctNames(names []string) []string {
	var out []string
	for _, n := range names {
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// paramPosition is where one params entry lands in the statement.
type paramPosition struct {
	display string
	sites   []adapters.PlaceholderSite
}

// matchPlaceholders pairs params with the statement's placeholders: positionally for `$n` and `?`, by name for
// ClickHouse. msg is the refusal text; it names no value.
func matchPlaceholders(statement string, params []string, sites []adapters.PlaceholderSite, style adapters.ParamStyle) (pos []paramPosition, msg string) {
	pos = make([]paramPosition, len(params))
	text := func(s adapters.PlaceholderSite) string { return statement[s.From:s.To] }
	switch style {
	case adapters.ParamDollar:
		used := map[int]bool{}
		for _, st := range sites {
			used[st.Index] = true
			if st.Index <= len(params) {
				pos[st.Index-1].sites = append(pos[st.Index-1].sites, st)
			}
		}
		for i := range params {
			pos[i].display = "$" + strconv.Itoa(i+1)
			if !used[i+1] {
				return nil, fmt.Sprintf("params lists %d names but the statement does not use %s; every $1 to $%d must appear", len(params), pos[i].display, len(params))
			}
		}
		for _, st := range sites {
			if st.Index > len(params) {
				return nil, fmt.Sprintf("the statement uses %s but params lists %d names", text(st), len(params))
			}
		}
	case adapters.ParamQuestion:
		if len(sites) != len(params) {
			return nil, fmt.Sprintf("the statement has %d placeholders but params lists %d names; one ? per entry, in order", len(sites), len(params))
		}
		for i, st := range sites {
			pos[i] = paramPosition{display: "? " + strconv.Itoa(i+1), sites: []adapters.PlaceholderSite{st}}
		}
	case adapters.ParamBraces:
		for i, name := range params {
			if slices.Contains(params[:i], name) {
				return nil, fmt.Sprintf("params lists %q twice; a ClickHouse placeholder is named once", name)
			}
		}
		for _, st := range sites {
			i := slices.Index(params, st.Name)
			if i < 0 {
				return nil, fmt.Sprintf("placeholder %s names no entry of params; the placeholder name must be a kept variable name", text(st))
			}
			if pos[i].display == "" {
				pos[i].display = text(st)
			}
			pos[i].sites = append(pos[i].sites, st)
		}
		for i, name := range params {
			if pos[i].display == "" {
				return nil, fmt.Sprintf("params lists %q but the statement has no {%s:Type} placeholder for it", name, name)
			}
		}
	}
	return pos, ""
}

// bind returns the binds with live values. Every name must still be the entry the statement was checked
// against; one that changed or left refuses, nothing ran. The values are copies for this one call.
func (s *Server) bind(ctx context.Context, kp *keptParams) ([]model.BindParam, error) {
	if kp == nil {
		return nil, nil
	}
	if s.cfg.Kept == nil {
		return nil, &keptChangedError{name: kp.names[0], gone: true}
	}
	scope := keptScope(ctx)
	now := make(map[string]keptvars.Entry, len(kp.uses))
	for _, u := range kp.uses {
		e, ok := s.cfg.Kept.Get(scope, u.Name)
		if !ok {
			return nil, &keptChangedError{name: u.Name, gone: true}
		}
		if e.ID != u.ID {
			return nil, &keptChangedError{name: u.Name}
		}
		now[u.Name] = e
	}
	out := make([]model.BindParam, len(kp.binds))
	for i, b := range kp.binds {
		b.Value = now[kp.names[i]].Value
		out[i] = b
	}
	return out, nil
}

// bindResult words a bind failure as the tool's refusal.
func bindResult(err error) (*mcp.CallToolResult, any, error) {
	return errResult(err.Error())
}

// queryKeptGone is the refusal for an approval wait cut short because a kept variable left the store.
func queryKeptGone(ctx context.Context) (res *mcp.CallToolResult, out any, err error, done bool) {
	var gone *keptGoneError
	if errors.As(context.Cause(ctx), &gone) {
		res, out, err = errResult(gone.queryText())
		return res, out, err, true
	}
	return nil, nil, nil, false
}
