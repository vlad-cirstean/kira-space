package scripts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/kirathecat/kira-studio/internal/configfs"
)

// FileExt is a run file's extension.
const FileExt = ".yaml"

// idRE is a valid run id and file stem. Lowercase UUIDs fit, so migrated ids keep their stems.
var idRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,79}$`)

// FileName returns the file name of a run id.
func FileName(id string) string { return id + FileExt }

// StemOf returns the run id of a file name and whether it is a valid id.
func StemOf(file string) (string, bool) {
	id := strings.TrimSuffix(file, FileExt)
	return id, idRE.MatchString(id)
}

// StringList is a YAML value that is one string or a list of strings; one string is one value.
type StringList []string

func (l *StringList) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*l = StringList{s}
		return nil
	}
	var list []string
	if err := json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("scripts: want a string or a list of strings")
	}
	*l = list
	return nil
}

func (l StringList) MarshalJSON() ([]byte, error) {
	if len(l) == 1 {
		return json.Marshal(l[0])
	}
	return json.Marshal([]string(l))
}

// File is a run file: the config of one run, snake_case in YAML. Id is the file stem.
type File struct {
	Name       string        `json:"name"`
	Kind       string        `json:"kind,omitempty"`
	Command    string        `json:"command,omitempty"`
	Prompt     string        `json:"prompt,omitempty"`
	Collection string        `json:"collection,omitempty"`
	Color      string        `json:"color,omitempty"`
	Dir        string        `json:"dir,omitempty"`
	Path       string        `json:"path,omitempty"`
	UseAdeDir  bool          `json:"use_ade_dir,omitempty"`
	Params     []FileParam   `json:"params,omitempty"`
	Smart      *FileSmart    `json:"smart,omitempty"`
	Schedule   *FileSchedule `json:"schedule,omitempty"`
}

// FileParam is one param in a file.
type FileParam struct {
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Options  []string   `json:"options,omitempty"`
	Default  StringList `json:"default,omitempty"`
	Required bool       `json:"required,omitempty"`
	Secret   bool       `json:"secret,omitempty"`
}

// FileSmart is the smart settings in a file.
type FileSmart struct {
	Model        string    `json:"model,omitempty"`
	MaxBudgetUSD float64   `json:"max_budget_usd,omitempty"`
	NoCostCap    bool      `json:"no_cost_cap,omitempty"`
	Timeout      string    `json:"timeout,omitempty"`
	Tools        []string  `json:"tools,omitzero"`
	BashPatterns []string  `json:"bash_patterns,omitempty"`
	MCP          []FileMCP `json:"mcp,omitempty"`
}

// FileMCP is one MCP server choice in a file.
type FileMCP struct {
	Server   string   `json:"server"`
	AllTools bool     `json:"all_tools,omitempty"`
	Tools    []string `json:"tools,omitempty"`
}

// FileSchedule is the schedule in a file. The task and branch of a Space run are state, not config.
type FileSchedule struct {
	Cron     string                `json:"cron"`
	Timezone string                `json:"timezone,omitempty"`
	Enabled  bool                  `json:"enabled"`
	Confirm  bool                  `json:"confirm"`
	Timeout  string                `json:"timeout,omitempty"`
	Params   map[string]StringList `json:"params,omitempty"`
}

// Fields converts the file to the validated shape. Conflicting command and prompt are the rule's job.
func (f File) Fields() CustomScriptFields {
	out := CustomScriptFields{
		Name: f.Name, Command: f.Command, Collection: f.Collection, Color: f.Color,
		DirMode: f.Dir, WorkingDir: f.Path, Kind: f.Kind, UseAdeDir: f.UseAdeDir,
	}
	if f.Kind == KindSmart {
		out.Command = f.Prompt
	}
	for _, p := range f.Params {
		out.Params = append(out.Params, Param{
			Name: p.Name, Type: p.Type, Options: p.Options, Default: p.Default, Required: p.Required, Secret: p.Secret,
		})
	}
	if f.Smart != nil {
		s := &Smart{
			Model: f.Smart.Model, MaxBudgetUSD: f.Smart.MaxBudgetUSD, NoCostCap: f.Smart.NoCostCap, Timeout: f.Smart.Timeout,
			Tools: f.Smart.Tools, BashPatterns: f.Smart.BashPatterns,
		}
		for _, m := range f.Smart.MCP {
			s.MCP = append(s.MCP, MCPChoice{Server: m.Server, AllTools: m.AllTools, Tools: m.Tools})
		}
		out.Smart = s
	}
	if f.Schedule != nil {
		sc := &Schedule{
			Cron: f.Schedule.Cron, Timezone: f.Schedule.Timezone, Enabled: f.Schedule.Enabled,
			Confirm: f.Schedule.Confirm, Timeout: f.Schedule.Timeout, Params: map[string][]string{},
		}
		for k, v := range f.Schedule.Params {
			sc.Params[k] = v
		}
		out.Schedule = sc
	}
	return out
}

// FileOf converts validated fields to a file; a normal field at its default is left out.
func FileOf(f CustomScriptFields) File {
	out := File{Name: f.Name, Kind: f.Kind, Collection: f.Collection, Dir: f.DirMode, Path: f.WorkingDir, UseAdeDir: f.UseAdeDir}
	if f.Color != "" && f.Color != "none" {
		out.Color = f.Color
	}
	if f.Kind == KindSmart {
		out.Prompt = f.Command
	} else {
		out.Command = f.Command
	}
	for _, p := range f.Params {
		out.Params = append(out.Params, FileParam{
			Name: p.Name, Type: p.Type, Options: nilIfEmpty(p.Options), Default: p.Default, Required: p.Required, Secret: p.Secret,
		})
	}
	if f.Smart != nil {
		s := &FileSmart{
			Model: f.Smart.Model, MaxBudgetUSD: f.Smart.MaxBudgetUSD, NoCostCap: f.Smart.NoCostCap, Timeout: f.Smart.Timeout,
			Tools: f.Smart.Tools, BashPatterns: nilIfEmpty(f.Smart.BashPatterns),
		}
		for _, m := range f.Smart.MCP {
			s.MCP = append(s.MCP, FileMCP{Server: m.Server, AllTools: m.AllTools, Tools: nilIfEmpty(m.Tools)})
		}
		out.Smart = s
	}
	if f.Schedule != nil {
		sc := &FileSchedule{
			Cron: f.Schedule.Cron, Timezone: f.Schedule.Timezone, Enabled: f.Schedule.Enabled,
			Confirm: f.Schedule.Confirm, Timeout: f.Schedule.Timeout,
		}
		for k, v := range f.Schedule.Params {
			if sc.Params == nil {
				sc.Params = map[string]StringList{}
			}
			sc.Params[k] = v
		}
		out.Schedule = sc
	}
	return out
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

// fileHeader is the comment at the top of a file Kira writes.
const fileHeader = "Kira run. Options: AGENTS.md in this folder."

// Render returns the YAML text of a file Kira writes.
func (f File) Render() ([]byte, error) { return configfs.RenderYAML(f, fileHeader) }

// DecodeJSON reads a projected config_json.
func DecodeJSON(js string) (File, error) {
	var f File
	dec := json.NewDecoder(bytes.NewReader([]byte(js)))
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("scripts: decode run config: %w", err)
	}
	return f, nil
}

// slug turns a name into a file stem: lowercase, runs of other characters become "-", at most 60.
func slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if r < 128 && (r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	if s == "" {
		return "run"
	}
	return s
}
