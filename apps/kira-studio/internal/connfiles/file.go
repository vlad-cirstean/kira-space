// Package connfiles keeps Kira Studio's connections as config files in <home>/connections. The files
// are the source of truth; the connections table and its mask rule rows are a projection written only
// by this package. A password or a URI stays out of the files: the file names a secret, whose sealed
// value lives in the app DB.
package connfiles

import (
	"regexp"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// Limits shared by the schema and the validator.
const (
	NameMax            = 120
	PortMax            = 65535
	ThrottleMin        = 0.01
	ThrottleMax        = 1000
	PreconnectMax      = 2000
	PreconnectDelayMax = 300
	DescriptionMax     = 1000
)

// Ext is the file suffix of a connection file.
const Ext = ".yaml"

// Defaults of keys a file leaves out.
const (
	ModeFields    = "fields"
	ModeURI       = "uri"
	ColorNone     = "none"
	McpRead       = "allow"
	McpWrite      = "prompt"
	McpDDL        = "deny"
	ReviewInherit = "inherit"
)

var (
	fileNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,59}\.yaml$`)
	stemRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,59}$`)
)

// File is <stem>.yaml. Keys left out take their default, so a value equal to its default is not written.
type File struct {
	Name           string         `json:"name"`
	Kind           string         `json:"kind"`
	Color          string         `json:"color,omitempty"`
	ReadOnly       bool           `json:"read_only,omitempty"`
	Mode           string         `json:"mode,omitempty"`
	Host           string         `json:"host,omitempty"`
	Port           int            `json:"port,omitempty"`
	Database       string         `json:"database,omitempty"`
	Username       string         `json:"username,omitempty"`
	Password       string         `json:"password,omitempty"`
	URI            string         `json:"uri,omitempty"`
	Options        map[string]any `json:"options,omitempty"`
	Preconnect     *Preconnect    `json:"preconnect,omitempty"`
	AutoExplain    bool           `json:"auto_explain,omitempty"`
	ThrottlePerSec float64        `json:"throttle_per_sec,omitempty"`
	MCP            *MCP           `json:"mcp,omitempty"`
	Masks          []Mask         `json:"masks,omitempty"`
	Tree           *Tree          `json:"tree,omitempty"`
	SchemaDDL      string         `json:"schema_ddl,omitempty"`
}

// Preconnect is the command run before a connect.
type Preconnect struct {
	Command  string `json:"command"`
	Sidecar  bool   `json:"sidecar,omitempty"`
	DelaySec int    `json:"delay_sec,omitempty"`
}

// MCP is what an AI client may do through the DB MCP server.
type MCP struct {
	Enabled       bool   `json:"enabled,omitempty"`
	Description   string `json:"description,omitempty"`
	Read          string `json:"read,omitempty"`
	Write         string `json:"write,omitempty"`
	DDL           string `json:"ddl,omitempty"`
	AutoExplain   *bool  `json:"auto_explain,omitempty"`
	ReviewResults string `json:"review_results,omitempty"`
	// StatementTimeoutSec nil means the default; a pointer so 0 (off) survives omitempty.
	StatementTimeoutSec *int `json:"statement_timeout_sec,omitempty"`
}

// Mask hides one column's values from the DB MCP server. KeepHint and Correlate default to true.
type Mask struct {
	Table     string `json:"table,omitempty"`
	Column    string `json:"column"`
	Kind      string `json:"kind"`
	KeepHint  *bool  `json:"keep_hint,omitempty"`
	Correlate *bool  `json:"correlate,omitempty"`
}

// Tree is the left panel's visibility for the connection.
type Tree struct {
	HiddenKinds []string `json:"hidden_kinds,omitempty"`
	HiddenPaths []string `json:"hidden_paths,omitempty"`
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// nonDefault returns &v when v differs from def, else nil (the key is left out).
func nonDefault(v, def bool) *bool {
	if v == def {
		return nil
	}
	return &v
}

// EffectiveMode is the file's mode with its default applied.
func (f File) EffectiveMode() string { return orDefault(f.Mode, ModeFields) }

// Fields is the file as the connection row's config columns. URI stays nil: a URI is never exposed.
func (f File) Fields() model.ConnectionFields {
	out := model.ConnectionFields{
		Name: f.Name, Kind: f.Kind, Color: orDefault(f.Color, ColorNone), Mode: f.EffectiveMode(), ReadOnly: f.ReadOnly,
		Host: nilIfEmpty(f.Host), Database: nilIfEmpty(f.Database), Username: nilIfEmpty(f.Username),
		Options: map[string]any{}, AutoExplain: f.AutoExplain, ThrottlePerSec: f.ThrottlePerSec,
		McpReadMode: McpRead, McpWriteMode: McpWrite, McpDdlMode: McpDDL, McpAutoExplain: true, McpReviewResults: ReviewInherit,
		McpStatementTimeoutSec: model.McpStatementTimeoutDefault,
	}
	if f.Port != 0 {
		p := f.Port
		out.Port = &p
	}
	for k, v := range f.Options {
		out.Options[k] = v
	}
	if p := f.Preconnect; p != nil {
		out.Preconnect = nilIfEmpty(p.Command)
		out.PreconnectSidecar, out.PreconnectDelaySec = p.Sidecar, p.DelaySec
	}
	if m := f.MCP; m != nil {
		out.McpEnabled, out.McpDescription = m.Enabled, m.Description
		out.McpReadMode, out.McpWriteMode, out.McpDdlMode = orDefault(m.Read, McpRead), orDefault(m.Write, McpWrite), orDefault(m.DDL, McpDDL)
		out.McpAutoExplain, out.McpReviewResults = boolOr(m.AutoExplain, true), orDefault(m.ReviewResults, ReviewInherit)
		if m.StatementTimeoutSec != nil {
			out.McpStatementTimeoutSec = *m.StatementTimeoutSec
		}
	}
	return out
}

// MaskFields is the mask as a rule row's fields.
func (m Mask) MaskFields() model.MaskRuleFields {
	return model.MaskRuleFields{
		TableName: orDefault(strings.TrimSpace(m.Table), "*"), ColumnName: strings.TrimSpace(m.Column), Kind: model.MaskKind(m.Kind),
		KeepHint: boolOr(m.KeepHint, true), Correlate: boolOr(m.Correlate, true) && m.Kind != string(model.MaskKindNumber),
	}
}

// MaskOf is a rule row's fields as a file mask, leaving defaults out.
func MaskOf(f model.MaskRuleFields) Mask {
	table := f.TableName
	if table == "*" {
		table = ""
	}
	return Mask{Table: table, Column: f.ColumnName, Kind: string(f.Kind), KeepHint: nonDefault(f.KeepHint, true), Correlate: nonDefault(f.Correlate, true)}
}

// Visibility is the tree section as the left panel's hidden sets, never nil slices.
func (f File) Visibility() model.TreeVisibility {
	v := model.EmptyVisibility()
	if f.Tree != nil {
		v.HiddenKinds = append(v.HiddenKinds, f.Tree.HiddenKinds...)
		v.HiddenPaths = append(v.HiddenPaths, f.Tree.HiddenPaths...)
	}
	return v
}

// TreeOf is the left panel's hidden sets as a tree section; nil when nothing is hidden.
func TreeOf(v model.TreeVisibility) *Tree {
	if len(v.HiddenKinds) == 0 && len(v.HiddenPaths) == 0 {
		return nil
	}
	return &Tree{HiddenKinds: v.HiddenKinds, HiddenPaths: v.HiddenPaths}
}

// FileOf builds the file of a stored connection. refs name the secrets of the password and the URI.
func FileOf(c model.ConnectionFields, masks []model.MaskRuleFields, tree model.TreeVisibility, ddl, passwordRef, uriRef string) File {
	f := File{
		Name: c.Name, Kind: c.Kind, ReadOnly: c.ReadOnly, Host: deref(c.Host), Database: deref(c.Database),
		Username: deref(c.Username), Password: passwordRef, URI: uriRef, AutoExplain: c.AutoExplain,
		ThrottlePerSec: c.ThrottlePerSec, SchemaDDL: ddl, Tree: TreeOf(tree),
	}
	if c.Color != ColorNone {
		f.Color = c.Color
	}
	if c.Mode != ModeFields {
		f.Mode = c.Mode
	}
	if c.Port != nil {
		f.Port = *c.Port
	}
	if len(c.Options) > 0 {
		f.Options = c.Options
	}
	if c.Preconnect != nil && *c.Preconnect != "" {
		f.Preconnect = &Preconnect{Command: *c.Preconnect, Sidecar: c.PreconnectSidecar, DelaySec: c.PreconnectDelaySec}
	}
	m := MCP{
		Enabled: c.McpEnabled, Description: c.McpDescription, AutoExplain: nonDefault(c.McpAutoExplain, true),
	}
	if c.McpReadMode != McpRead {
		m.Read = c.McpReadMode
	}
	if c.McpWriteMode != McpWrite {
		m.Write = c.McpWriteMode
	}
	if c.McpDdlMode != McpDDL {
		m.DDL = c.McpDdlMode
	}
	if c.McpReviewResults != ReviewInherit {
		m.ReviewResults = c.McpReviewResults
	}
	if c.McpStatementTimeoutSec != model.McpStatementTimeoutDefault {
		v := c.McpStatementTimeoutSec
		m.StatementTimeoutSec = &v
	}
	if m != (MCP{}) {
		f.MCP = &m
	}
	for _, r := range masks {
		f.Masks = append(f.Masks, MaskOf(r))
	}
	return f
}

// SecretName reads the name out of a whole-value `${secret:name}` reference; "" when ref is not one.
func SecretName(ref string) string {
	refs, p := configfs.Refs(ref)
	if p != nil || len(refs) != 1 || refs[0].Kind != configfs.RefSecret || refs[0].Start != 0 || refs[0].End != len(ref) {
		return ""
	}
	return refs[0].Name
}

// RefOf is the `${secret:name}` reference of a secret name.
func RefOf(name string) string { return "${secret:" + name + "}" }

// FileName is the file name of a stem.
func FileName(stem string) string { return stem + Ext }

// StemOf strips the suffix from file and reports whether the rest is a valid stem.
func StemOf(file string) (string, bool) {
	stem, ok := strings.CutSuffix(file, Ext)
	return stem, ok && stemRe.MatchString(stem)
}
