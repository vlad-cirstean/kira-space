package agentport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tailscale/hujson"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// mask replaces a credential in a shown diff.
const mask = "••••"

// ExportFile is one file an export would write, with masked text for the diff.
type ExportFile struct {
	Role     string `json:"role"`
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Changed  bool   `json:"changed"`
	Before   string `json:"before"`
	After    string `json:"after"`
	Language string `json:"language"`
}

// ExportServer is one MCP server an export would register with the provider's CLI.
type ExportServer struct {
	Name      string `json:"name"`
	Action    string `json:"action"`
	Transport string `json:"transport"`
	Target    string `json:"target"`
}

// Skipped is a setting an export leaves out, with the reason.
type Skipped struct {
	What   string `json:"what"`
	Reason string `json:"reason"`
}

// ExportPreview is what an export would write, for the user to confirm.
type ExportPreview struct {
	Token      string         `json:"token"`
	Provider   string         `json:"provider"`
	Files      []ExportFile   `json:"files"`
	Servers    []ExportServer `json:"servers"`
	EnvNames   []string       `json:"envNames"`
	Skipped    []Skipped      `json:"skipped"`
	Notes      []string       `json:"notes"`
	CLIMissing bool           `json:"cliMissing"`
}

// exportFile is a file's computed output, kept server side.
type exportFile struct {
	role, path string
	exists     bool
	before     []byte
	after      []byte
	hash       string
	mode       os.FileMode
	defMode    os.FileMode
	// norm maps a file's bytes to what the staleness hash covers.
	norm func([]byte) []byte
}

func (f *exportFile) changed() bool { return !bytes.Equal(f.before, f.after) }

type exportServer struct {
	row  ExportServer
	json []byte
}

type exportSession struct {
	provider string
	files    []*exportFile
	servers  []exportServer
	cliEnv   []string
	// claudeJSON is the resolved ~/.claude.json, backed up before the CLI edits it.
	claudeJSON string
}

// exporter collects what an export leaves out and what the user must set.
type exporter struct {
	svc      *Service
	skipped  []Skipped
	notes    []string
	envNames map[string]bool
	secrets  map[string]string // env name -> secret name
	// collision names two secrets that derive one variable: first, second, variable.
	collision [3]string
}

func (ex *exporter) skip(what, reason string) { ex.skipped = append(ex.skipped, Skipped{what, reason}) }

func hashOf(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// secretEnv is the variable a secret travels as; two secrets deriving one name are an error.
func (ex *exporter) secretEnv(name string) string {
	env := secretEnvName(name)
	if prev, ok := ex.secrets[env]; ok && prev != name {
		ex.collision = [3]string{prev, name, env}
	}
	ex.secrets[env] = name
	return env
}

// safe checks one reference field value for export: a credential literal blocks the item, a $${ escape
// has no form in the provider. conv writes the provider's reference syntax.
func (ex *exporter) safe(what string, l loc, name, value string, conv func(string, func(string) string) (string, bool), agent string) (string, bool) {
	if hasSecret(classify(l, name, value)) {
		ex.skip(what, i18n.T("backend.core.agentPort.export.plaintext.body", nil))
		return "", false
	}
	out, ok := conv(value, ex.secretEnv)
	if !ok {
		ex.skip(what, i18n.T("backend.core.agentPort.export.noForm.body", map[string]string{"agent": agent}))
		return "", false
	}
	refs, _ := configfs.Refs(value)
	for _, r := range refs {
		if r.Kind == configfs.RefSecret {
			ex.envNames[secretEnvName(r.Name)] = true
		} else {
			ex.envNames[r.Name] = true
		}
	}
	return out, true
}

// claudeRefs writes settings text as Claude config text: ${secret:x} becomes ${KIRA_SECRET_X}.
func claudeRefs(text string, secretEnv func(string) string) (string, bool) {
	if strings.Contains(text, "$${") {
		return "", false
	}
	refs, p := configfs.Refs(text)
	if p != nil {
		return "", false
	}
	var b strings.Builder
	last := 0
	for _, r := range refs {
		b.WriteString(text[last:r.Start])
		name := r.Name
		if r.Kind == configfs.RefSecret {
			name = secretEnv(r.Name)
		}
		b.WriteString("${" + name + "}")
		last = r.End
	}
	b.WriteString(text[last:])
	return b.String(), true
}

// readFor reads the target for export: bytes, mode, and the resolved symlink target.
func (ex *exporter) readFor(t target, defMode os.FileMode, max int64) (*exportFile, error) {
	real, err := resolveTarget(t.Path)
	if err != nil {
		return nil, ipcerr.InternalErr(err)
	}
	data, mode, exists, err := readIfExists(real, max)
	if err != nil {
		return nil, ipcerr.BadRequest(readProblem(real, err, max))
	}
	return &exportFile{role: t.Role, path: real, exists: exists, before: data, hash: hashOf(data), mode: mode, defMode: defMode}, nil
}

// maskText hides every credential-looking string value of a JSON text in the shown diff: the user's own
// file may hold credentials, so the shown diff is never the written bytes.
func maskText(text string) string {
	std, err := hujson.Standardize([]byte(text))
	if err != nil {
		return text
	}
	var secretsFound []string
	collectSecrets(std, &secretsFound)
	slices.SortFunc(secretsFound, func(a, b string) int { return len(b) - len(a) })
	for _, s := range secretsFound {
		text = strings.ReplaceAll(text, s, mask)
		if q := quoteInner(s); q != s {
			text = strings.ReplaceAll(text, q, mask)
		}
	}
	return text
}

// ExportPreviewArgs names the provider to render for.
type ExportPreviewArgs struct {
	Provider string `json:"provider"`
}

// ExportApplyArgs applies a preview: Files lists the roles to write, Servers whether to register the
// MCP servers.
type ExportApplyArgs struct {
	Token   string   `json:"token"`
	Files   []string `json:"files"`
	Servers bool     `json:"servers"`
}

// ExportPreview renders the app's settings and context as the provider's native files and shows the
// diff. It writes nothing.
func (s *Service) ExportPreview(ctx context.Context, args ExportPreviewArgs) (ExportPreview, error) {
	files, configDir, ok := s.targets(ctx, args.Provider)
	if !ok || !s.has(args.Provider) || s.o.Settings == nil || s.o.Context == nil {
		return ExportPreview{}, ipcerr.BadRequest(i18n.T("backend.core.agentPort.unknownProvider.body", map[string]string{"provider": args.Provider}))
	}
	sv, cv := s.o.Settings.View(), s.o.Context.View()
	if sv.Problem != nil || !sv.Editable {
		return ExportPreview{}, ipcerr.BadRequest(i18n.T("backend.core.agentPort.fixSettings.body", nil))
	}
	if cv.Problem != nil || !cv.Editable {
		return ExportPreview{}, ipcerr.BadRequest(i18n.T("backend.core.agentPort.fixContext.body", nil))
	}
	ex := &exporter{svc: s, envNames: map[string]bool{}, secrets: map[string]string{}}
	sess := &exportSession{provider: args.Provider}
	var out ExportPreview
	var err error
	switch args.Provider {
	case providerClaude:
		out.CLIMissing, err = s.exportClaude(ctx, ex, sess, files, configDir, filepath.Dir(sv.Path))
	default:
		err = s.exportOpenCode(ex, sess, files)
	}
	if err != nil {
		return ExportPreview{}, err
	}
	if c := ex.collision; c[0] != "" {
		return ExportPreview{}, ipcerr.BadRequest(i18n.T("backend.core.agentPort.export.collision.body", map[string]string{"a": c[0], "b": c[1], "name": c[2]}))
	}
	if err := s.exportContext(ex, sess, files, cv.Text); err != nil {
		return ExportPreview{}, err
	}
	out.Provider = args.Provider
	out.Files = make([]ExportFile, 0, len(sess.files))
	for _, f := range sess.files {
		lang := "json"
		if f.role == roleContext {
			lang = "markdown"
		}
		out.Files = append(out.Files, ExportFile{
			Role: f.role, Path: s.display(f.path), Exists: f.exists, Changed: f.changed(), Language: lang,
			Before: displayText(f.role, f.before), After: displayText(f.role, f.after),
		})
	}
	out.Servers = make([]ExportServer, 0, len(sess.servers))
	for _, sv := range sess.servers {
		out.Servers = append(out.Servers, sv.row)
	}
	out.EnvNames = sortedKeys(ex.envNames)
	out.Skipped, out.Notes = ex.skipped, ex.notes
	if out.Skipped == nil {
		out.Skipped = []Skipped{}
	}
	if out.Notes == nil {
		out.Notes = []string{}
	}
	if out.EnvNames == nil {
		out.EnvNames = []string{}
	}
	out.Token = s.sessions.put(&session{exp: sess})
	return out, nil
}

// displayText is a file's text for the diff: JSON is masked, context is the user's own prose.
func displayText(role string, b []byte) string {
	if role == roleContext {
		return string(b)
	}
	return maskText(string(b))
}

// exportContext adds the context file: replaced whole by context.md; an empty context.md leaves the
// user's file alone.
func (s *Service) exportContext(ex *exporter, sess *exportSession, files []target, text string) error {
	for _, t := range files {
		if t.Role != roleContext {
			continue
		}
		f, err := ex.readFor(t, 0o644, maxContextFile)
		if err != nil {
			return err
		}
		if strings.TrimSpace(text) == "" {
			f.after = f.before
			ex.notes = append(ex.notes, i18n.T("backend.core.agentPort.export.noContext.body", map[string]string{"file": filepath.Base(t.Path)}))
		} else {
			f.after = []byte(text)
		}
		sess.files = append(sess.files, f)
	}
	return nil
}

// ExportApply writes the confirmed files, each after a backup, then registers the MCP servers. A file
// changed since the preview is a conflict and nothing is written. The first failure stops; the backups
// are the undo.
func (s *Service) ExportApply(ctx context.Context, args ExportApplyArgs) (PortResult, error) {
	sess, ok := s.sessions.take(args.Token)
	if !ok || sess.exp == nil {
		return PortResult{}, expired()
	}
	exp := sess.exp
	res := PortResult{Written: []string{}, Backups: []string{}, Servers: []ServerResult{}, Secrets: []string{}, Notes: []string{}}
	var chosen []*exportFile
	for _, f := range exp.files {
		if slices.Contains(args.Files, f.role) && f.changed() {
			chosen = append(chosen, f)
		}
	}
	for _, f := range chosen {
		cur, _, _, err := readIfExists(f.path, 1<<30)
		if err != nil || hashOf(f.normalize(cur)) != hashOf(f.normalize(f.before)) {
			return PortResult{}, stale()
		}
	}
	now := s.o.Now()
	for _, f := range chosen {
		if f.exists {
			b, err := backup(f.path, f.before, f.mode, now)
			if err != nil {
				return res, ipcerr.InternalErr(err)
			}
			res.Backups = append(res.Backups, b)
		}
		if err := writeFile(f.path, f.after, f.defMode); err != nil {
			return res, ipcerr.InternalErr(err)
		}
		res.Written = append(res.Written, f.path)
	}
	if args.Servers && len(exp.servers) > 0 {
		s.applyServers(ctx, exp, &res, now)
	}
	return res, nil
}

func (f *exportFile) normalize(b []byte) []byte {
	if f.norm != nil {
		return f.norm(b)
	}
	return b
}

func quoteInner(s string) string {
	b, err := json.Marshal(s)
	if err != nil || len(b) < 2 {
		return s
	}
	return string(b[1 : len(b)-1])
}

// collectSecrets gathers every string of a JSON document that looks like a credential: its key names
// one, or its value matches a credential pattern. References are not credentials.
func collectSecrets(std []byte, out *[]string) {
	var doc any
	if json.Unmarshal(std, &doc) != nil {
		return
	}
	var walk func(key string, v any)
	walk = func(key string, v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, e := range t {
				walk(k, e)
			}
		case []any:
			for _, e := range t {
				walk(key, e)
			}
		case string:
			if len(t) >= 6 && !strings.Contains(t, "${") && !strings.Contains(t, "{env:") && (valueIsSecret(t) || nameIsSecret(key)) && !slices.Contains(*out, t) {
				*out = append(*out, t)
			}
		}
	}
	walk("", doc)
}
