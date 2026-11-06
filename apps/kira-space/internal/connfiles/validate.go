package connfiles

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// ValidationError is the first rule a file breaks. Path is the JSON pointer of the field, Key the
// catalog key of the message, Args its values. The message never echoes a field value.
type ValidationError struct {
	Path string
	Key  string
	Args map[string]string
}

func (e *ValidationError) Error() string { return i18n.T(e.Key, e.Args) }

func bad(path, key string, args map[string]string) *ValidationError {
	return &ValidationError{Path: path, Key: key, Args: args}
}

func invalid(path string) *ValidationError {
	return bad(path, "backend.core.config.invalid.body", map[string]string{"field": strings.TrimPrefix(strings.ReplaceAll(path, "/", "."), ".")})
}

func notUsed(path, why string) *ValidationError {
	return bad(path, "backend.studio.connectionFiles.fieldNotUsed.body", map[string]string{"field": strings.TrimPrefix(strings.ReplaceAll(path, "/", "."), "."), "why": why})
}

// secretOptionKeys are query parameters whose value is a secret a driver honours: pgx's password and key
// passphrase, Mongo's key-file passphrase and proxy password. A URI keeps them inside its secret;
// fields mode has no slot for them, so an options key in this set is refused.
var secretOptionKeys = map[string]bool{
	"password": true, "sslpassword": true, "tlscertificatekeyfilepassword": true, "proxypassword": true,
}

// SecretOptionKey returns the first (sorted) key of options that holds a secret.
func SecretOptionKey(options map[string]any) (string, bool) {
	var found []string
	for k := range options {
		if secretOptionKeys[strings.ToLower(k)] {
			found = append(found, k)
		}
	}
	if len(found) == 0 {
		return "", false
	}
	sort.Strings(found)
	return found[0], true
}

var (
	fileKinds     = map[string]bool{"sqlite": true}
	awsStyleKinds = map[string]bool{"sqs": true, "s3": true}
)

// FileKind reports whether the kind's `database` is a local file path.
func FileKind(kind string) bool { return fileKinds[kind] }

// Validate runs every rule on the file. The decoder runs it as the file's cross-field rule, and Create
// runs it before it writes a file.
func (f File) Validate() *ValidationError {
	for _, check := range []func() *ValidationError{f.checkIdentity, f.checkLimits, f.checkMCP, f.checkMode, f.checkMasks, f.checkMaskReview, f.checkTree} {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

func (f File) checkIdentity() *ValidationError {
	name := strings.TrimSpace(f.Name)
	if name == "" || model.UTF16Len(name) > NameMax {
		return bad("/name", "backend.studio.connection.nameLength.body", nil)
	}
	if !model.ValidConnectionKind(f.Kind) {
		return invalid("/kind")
	}
	if f.Color != "" && !model.ValidConnectionColor(f.Color) {
		return invalid("/color")
	}
	if f.Mode != "" && !model.ValidConnectionMode(f.Mode) {
		return invalid("/mode")
	}
	return nil
}

func (f File) checkLimits() *ValidationError {
	if f.Port < 0 || f.Port > PortMax {
		return bad("/port", "backend.studio.connection.portRange.body", nil)
	}
	if p := f.Preconnect; p != nil {
		if strings.TrimSpace(p.Command) == "" || len(p.Command) > PreconnectMax {
			return bad("/preconnect/command", "backend.studio.connection.preconnectLength.body", nil)
		}
		if p.DelaySec < 0 || p.DelaySec > PreconnectDelayMax {
			return bad("/preconnect/delay_sec", "backend.studio.connection.preconnectDelayRange.body", nil)
		}
	}
	if math.IsNaN(f.ThrottlePerSec) || math.IsInf(f.ThrottlePerSec, 0) {
		return bad("/throttle_per_sec", "backend.studio.connection.throttleFinite.body", nil)
	}
	if f.ThrottlePerSec != 0 && (f.ThrottlePerSec < ThrottleMin || f.ThrottlePerSec > ThrottleMax) {
		return bad("/throttle_per_sec", "backend.studio.connection.throttleRange.body", nil)
	}
	return nil
}

func (f File) checkMCP() *ValidationError {
	m := f.MCP
	if m == nil {
		return nil
	}
	if m.Read != "" && !model.ValidMcpPermissionMode(m.Read) {
		return invalid("/mcp/read")
	}
	if m.Write != "" && !model.ValidMcpPermissionMode(m.Write) {
		return invalid("/mcp/write")
	}
	if m.DDL != "" && !model.ValidMcpPermissionMode(m.DDL) {
		return invalid("/mcp/ddl")
	}
	if m.Masked != "" && !model.ValidMcpMaskedMode(m.Masked) {
		return invalid("/mcp/masked")
	}
	if m.ReviewResults != "" && !model.ValidMcpReviewResults(m.ReviewResults) {
		return invalid("/mcp/review_results")
	}
	if r := m.Review; r != nil {
		for path, v := range map[string]string{"/mcp/review/read": r.Read, "/mcp/review/write": r.Write, "/mcp/review/ddl": r.DDL, "/mcp/review/masked": r.Masked} {
			if v != "" && !model.ValidMcpReviewResults(v) {
				return invalid(path)
			}
		}
	}
	if m.StatementTimeoutSec != nil && !model.ValidMcpStatementTimeout(*m.StatementTimeoutSec) {
		return bad("/mcp/statement_timeout_sec", "backend.studio.connection.mcpStatementTimeoutRange.body", nil)
	}
	if m.Tables != nil {
		if err := f.checkMCPTables(*m.Tables); err != nil {
			return err
		}
	}
	if len(m.Description) > DescriptionMax {
		return bad("/mcp/description", "backend.studio.connection.mcpDescriptionLength.body", nil)
	}
	return nil
}

// MCPTablesMax caps the entries of mcp.tables.
const MCPTablesMax = 10000

func (f File) checkMCPTables(entries []string) *ValidationError {
	if !tableref.Applies(f.Kind) {
		return bad("/mcp/tables", "backend.studio.connection.mcpTablesKind.body", nil)
	}
	if len(entries) > MCPTablesMax {
		return bad("/mcp/tables", "backend.studio.connection.mcpTablesMax.body", map[string]string{"max": strconv.Itoa(MCPTablesMax)})
	}
	for i, e := range entries {
		if _, err := tableref.Parse(f.Kind, e); err != nil {
			return bad(fmt.Sprintf("/mcp/tables/%d", i), "backend.studio.connection.mcpTablesEntry.body", nil)
		}
	}
	return nil
}

// checkMode checks the shape of fields mode against uri mode and the kind.
func (f File) checkMode() *ValidationError {
	if f.EffectiveMode() == ModeURI {
		return f.checkURIMode()
	}
	return f.checkFieldsMode()
}

func (f File) checkURIMode() *ValidationError {
	if f.URI == "" {
		return bad("/uri", "backend.studio.connection.uriRequired.body", nil)
	}
	for _, c := range []struct{ path, val string }{
		{"/host", f.Host}, {"/database", f.Database}, {"/username", f.Username}, {"/password", f.Password},
	} {
		if c.val != "" {
			return notUsed(c.path, "uri mode")
		}
	}
	if f.Port != 0 {
		return notUsed("/port", "uri mode")
	}
	return nil
}

func (f File) checkFieldsMode() *ValidationError {
	if f.URI != "" {
		return notUsed("/uri", "fields mode")
	}
	switch {
	case fileKinds[f.Kind]:
		path := strings.TrimSpace(f.Database)
		if path == "" {
			return bad("/database", "backend.studio.connection.databaseFileRequired.body", nil)
		}
		if !strings.HasPrefix(path, "/") {
			return bad("/database", "backend.studio.connection.databasePathAbsolute.body", nil)
		}
	case awsStyleKinds[f.Kind]:
		if f.Host != "" {
			return notUsed("/host", f.Kind)
		}
		if f.Port != 0 {
			return notUsed("/port", f.Kind)
		}
	default:
		if f.Host == "" {
			return bad("/host", "backend.studio.connection.hostRequired.body", nil)
		}
		if f.Port == 0 {
			return bad("/port", "backend.studio.connection.portRequired.body", nil)
		}
	}
	if f.Password != "" && (fileKinds[f.Kind] || awsStyleKinds[f.Kind]) {
		return notUsed("/password", f.Kind)
	}
	if key, found := SecretOptionKey(f.Options); found {
		return bad("/options/"+strings.NewReplacer("~", "~0", "/", "~1").Replace(key), "backend.studio.connection.secretOption.body", map[string]string{"option": key})
	}
	return nil
}

func (f File) checkMasks() *ValidationError {
	seen := map[string]bool{}
	for i, m := range f.Masks {
		at := fmt.Sprintf("/masks/%d", i)
		if strings.TrimSpace(m.Column) == "" {
			return bad(at+"/column", "backend.studio.connectionFiles.maskColumn.body", nil)
		}
		if !model.ValidMaskKind(m.Kind) {
			return invalid(at + "/kind")
		}
		if m.Kind == string(model.MaskKindNumber) && m.Correlate != nil && *m.Correlate {
			return bad(at+"/correlate", "backend.studio.connectionFiles.maskNumber.body", nil)
		}
		key := strings.ToLower(orDefault(strings.TrimSpace(m.Table), "*")) + "\x00" + strings.ToLower(strings.TrimSpace(m.Column))
		if seen[key] {
			return bad(at+"/column", "backend.studio.connectionFiles.maskDuplicate.body", map[string]string{"column": m.Column})
		}
		seen[key] = true
	}
	return nil
}

// MaskReviewMax caps the entries of mask_review.columns.
const MaskReviewMax = 100000

func (f File) checkMaskReview() *ValidationError {
	r := f.MaskReview
	if r == nil {
		return nil
	}
	if !tableref.Applies(f.Kind) || f.Kind == "mongodb" || f.Kind == "kibana" {
		return bad("/mask_review", "backend.studio.connection.maskReviewKind.body", nil)
	}
	if len(r.Columns) > MaskReviewMax {
		return bad("/mask_review", "backend.studio.connection.maskReviewMax.body", map[string]string{"max": strconv.Itoa(MaskReviewMax)})
	}
	for i, e := range r.Columns {
		if _, err := tableref.ParseColumn(f.Kind, e); err != nil {
			return bad(fmt.Sprintf("/mask_review/columns/%d", i), "backend.studio.connection.maskReviewEntry.body", nil)
		}
	}
	return nil
}

func (f File) checkTree() *ValidationError {
	t := f.Tree
	if t == nil {
		return nil
	}
	seen := map[string]bool{}
	for i, k := range t.HiddenKinds {
		if seen[k] {
			return bad(fmt.Sprintf("/tree/hidden_kinds/%d", i), "backend.studio.connectionFiles.treeDuplicate.body", nil)
		}
		seen[k] = true
	}
	seen = map[string]bool{}
	for i, p := range t.HiddenPaths {
		if strings.TrimSpace(p) == "" {
			return bad(fmt.Sprintf("/tree/hidden_paths/%d", i), "backend.studio.connectionFiles.treePathBlank.body", nil)
		}
		if seen[p] {
			return bad(fmt.Sprintf("/tree/hidden_paths/%d", i), "backend.studio.connectionFiles.treeDuplicate.body", nil)
		}
		seen[p] = true
	}
	return nil
}

// Problem is the error as a decode problem of a file.
func (e *ValidationError) Problem() *configfs.Problem {
	return &configfs.Problem{Path: e.Path, Message: e.Error()}
}
