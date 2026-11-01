package scripts

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// Script kinds.
const (
	KindScript = "script"
	KindSmart  = "smart"
)

// Param types.
const (
	ParamText        = "text"
	ParamSelect      = "select"
	ParamMultiselect = "multiselect"
)

// Smart defaults (D1).
const (
	DefaultBudgetUSD = 1.0
	DefaultTimeout   = "15m"
)

// Bounds.
const (
	MaxPrompt       = 20000
	MaxParams       = 20
	MaxOptions      = 50
	MaxOptionRunes  = 200
	MaxTextValue    = 2000
	MaxPatterns     = 30
	MaxPatternRunes = 200
	MaxMCPServers   = 10
	MaxMCPTools     = 100
	MinBudgetUSD    = 0.05
	MaxBudgetUSD    = 25.0
	MinTimeout      = time.Minute
	MaxTimeout      = 2 * time.Hour
)

// BuiltinTools are the Claude built-ins a smart script may tick.
var BuiltinTools = []string{"Read", "Grep", "Glob", "Edit", "Write", "NotebookEdit", "Bash"}

// DefaultTools are ticked for a new smart script.
var DefaultTools = []string{"Read", "Grep", "Glob"}

// BuiltinVars are the variables a workflow run has; reserved as param names in both apps.
var BuiltinVars = []string{"task", "jira", "repo", "branch", "base", "worktree"}

// SecretMask is what a run records in place of a secret value.
const SecretMask = secrets.Mask

var (
	paramNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	mcpNameRE   = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	varRE       = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)
)

// Param is one input of a script.
type Param struct {
	Name string `json:"name"`
	// Type is text, select or multiselect.
	Type    string   `json:"type"`
	Options []string `json:"options"`
	Default []string `json:"default"`
	// Required means the run dialog needs a value.
	Required bool `json:"required"`
	// Secret (text only) reaches the run through env only: never the prompt, never a stored run.
	// A saved value lives sealed in secret_values; the record holds names only.
	Secret bool `json:"secret"`
}

// MCPChoice is one user MCP server and the tools of it the script may call: all of them, or the
// named Tools.
type MCPChoice struct {
	Server   string   `json:"server"`
	AllTools bool     `json:"allTools"`
	Tools    []string `json:"tools"`
}

// Smart is the headless Claude setup of a smart script.
type Smart struct {
	Model        string      `json:"model"`
	MaxBudgetUSD float64     `json:"maxBudgetUsd"`
	Timeout      string      `json:"timeout"`
	Tools        []string    `json:"tools"`
	BashPatterns []string    `json:"bashPatterns"`
	MCP          []MCPChoice `json:"mcp"`
}

// TimeoutDuration parses Timeout; Validate has already bounded it.
func (s Smart) TimeoutDuration() time.Duration {
	d, err := time.ParseDuration(s.Timeout)
	if err != nil {
		return 15 * time.Minute
	}
	return d
}

// orList renders "a, b or c".
func orList(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func validParams(params []Param, kind, prompt string) ([]Param, error) {
	if len(params) > MaxParams {
		return nil, invalid("scripts: at most %d params", MaxParams)
	}
	out := make([]Param, 0, len(params))
	seen := map[string]bool{}
	for _, p := range params {
		p.Name = strings.TrimSpace(p.Name)
		switch {
		case !paramNameRE.MatchString(p.Name):
			return nil, invalid("scripts: param name %q must be lowercase letters, digits and _, starting with a letter, up to 32 characters", p.Name)
		case containsStr(BuiltinVars, p.Name):
			return nil, invalid("scripts: param name %q is reserved", p.Name)
		case seen[p.Name]:
			return nil, invalid("scripts: param %q is listed twice", p.Name)
		}
		seen[p.Name] = true
		var err error
		if p, err = validParamShape(p); err != nil {
			return nil, err
		}
		if p.Secret && kind == KindSmart && containsVar(prompt, p.Name) {
			return nil, invalid("scripts: secret param %s cannot be used in the prompt", p.Name)
		}
		out = append(out, p)
	}
	return out, nil
}

func validParamShape(p Param) (Param, error) {
	p.Options = nonNil(p.Options)
	p.Default = nonNil(p.Default)
	switch p.Type {
	case ParamText:
		return p, validTextParam(p)
	case ParamSelect, ParamMultiselect:
		return p, validChoiceParam(p)
	}
	return p, invalid("scripts: param %s: type must be text, select or multiselect", p.Name)
}

func validTextParam(p Param) error {
	if len(p.Options) > 0 {
		return invalid("scripts: param %s: a text param has no options", p.Name)
	}
	if len(p.Default) > 1 {
		return invalid("scripts: param %s: a text param has one default", p.Name)
	}
	if len(p.Default) == 1 {
		if p.Secret {
			return invalid("scripts: secret param %s cannot have a default", p.Name)
		}
		if err := checkText(p.Default[0]); err != nil {
			return invalid("scripts: param %s: default %v", p.Name, err)
		}
	}
	return nil
}

// validChoiceParam checks a select or multiselect param; it trims p.Options in place.
func validChoiceParam(p Param) error {
	if p.Secret {
		return invalid("scripts: param %s: only a text param can be secret", p.Name)
	}
	if len(p.Options) == 0 || len(p.Options) > MaxOptions {
		return invalid("scripts: param %s: give 1 to %d options", p.Name, MaxOptions)
	}
	seen := map[string]bool{}
	for i, o := range p.Options {
		o = strings.TrimSpace(o)
		p.Options[i] = o
		switch {
		case o == "" || utf8.RuneCountInString(o) > MaxOptionRunes || strings.ContainsAny(o, "\r\n"):
			return invalid("scripts: param %s: an option must be 1 to %d characters on one line", p.Name, MaxOptionRunes)
		case seen[o]:
			return invalid("scripts: param %s: option %q is listed twice", p.Name, o)
		}
		seen[o] = true
	}
	if p.Type == ParamSelect && len(p.Default) > 1 {
		return invalid("scripts: param %s: a select param has one default", p.Name)
	}
	for _, d := range p.Default {
		if !containsStr(p.Options, d) {
			return invalid("scripts: param %s: default %q is not an option", p.Name, d)
		}
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// checkText bounds a text value: no NUL or control characters except newline and tab.
func checkText(v string) error {
	if utf8.RuneCountInString(v) > MaxTextValue {
		return fmt.Errorf("is longer than %d characters", MaxTextValue)
	}
	for _, r := range v {
		if r == 0 || (unicode.IsControl(r) && r != '\n' && r != '\t') {
			return fmt.Errorf("has a control character")
		}
	}
	return nil
}

func containsVar(text, name string) bool {
	for _, m := range varRE.FindAllStringSubmatch(text, -1) {
		if m[1] == name {
			return true
		}
	}
	return false
}

// validSmart fills defaults and bounds s; models are the aliases a smart script may name.
func validSmart(s *Smart, models harness.Catalog) (*Smart, error) {
	if s == nil {
		s = &Smart{}
	}
	out := *s
	if out.Model == "" {
		out.Model = models.Default
	}
	if !models.Valid(out.Model) {
		return nil, invalid("scripts: model must be %s", orList(models.Aliases))
	}
	if out.MaxBudgetUSD == 0 {
		out.MaxBudgetUSD = DefaultBudgetUSD
	}
	if out.MaxBudgetUSD < MinBudgetUSD || out.MaxBudgetUSD > MaxBudgetUSD {
		return nil, invalid("scripts: budget must be between %.2f and %.0f USD", MinBudgetUSD, MaxBudgetUSD)
	}
	out.Timeout = strings.TrimSpace(out.Timeout)
	if out.Timeout == "" {
		out.Timeout = DefaultTimeout
	}
	d, err := time.ParseDuration(out.Timeout)
	if err != nil || d < MinTimeout || d > MaxTimeout {
		return nil, invalid("scripts: timeout must be a duration between 1m and 2h, like 15m")
	}
	if out.Tools == nil {
		out.Tools = append([]string{}, DefaultTools...)
	}
	picked := map[string]bool{}
	for _, t := range out.Tools {
		if !containsStr(BuiltinTools, t) {
			return nil, invalid("scripts: unknown tool %q", t)
		}
		picked[t] = true
	}
	ordered := []string{}
	for _, t := range BuiltinTools {
		if picked[t] {
			ordered = append(ordered, t)
		}
	}
	out.Tools = ordered
	if out.BashPatterns, err = validPatterns(out.BashPatterns, picked["Bash"]); err != nil {
		return nil, err
	}
	if out.MCP, err = validMCP(out.MCP); err != nil {
		return nil, err
	}
	return &out, nil
}

func validPatterns(patterns []string, bash bool) ([]string, error) {
	out := []string{}
	if len(patterns) > 0 && !bash {
		return nil, invalid("scripts: Bash patterns need the Bash tool")
	}
	if len(patterns) > MaxPatterns {
		return nil, invalid("scripts: at most %d Bash patterns", MaxPatterns)
	}
	seen := map[string]bool{}
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		switch {
		case p == "" || utf8.RuneCountInString(p) > MaxPatternRunes:
			return nil, invalid("scripts: a Bash pattern must be 1 to %d characters", MaxPatternRunes)
		case strings.ContainsAny(p, ")\r\n"):
			return nil, invalid("scripts: a Bash pattern cannot contain ) or a new line")
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

func validMCP(choices []MCPChoice) ([]MCPChoice, error) {
	out := []MCPChoice{}
	if len(choices) > MaxMCPServers {
		return nil, invalid("scripts: at most %d MCP servers", MaxMCPServers)
	}
	seen := map[string]bool{}
	for _, c := range choices {
		if !mcpNameRE.MatchString(c.Server) {
			return nil, invalid("scripts: MCP server name %q is not valid", c.Server)
		}
		if seen[c.Server] {
			return nil, invalid("scripts: MCP server %s is listed twice", c.Server)
		}
		seen[c.Server] = true
		if c.AllTools {
			out = append(out, MCPChoice{Server: c.Server, AllTools: true, Tools: []string{}})
			continue
		}
		if len(c.Tools) == 0 {
			return nil, invalid("scripts: tick at least one tool of %s or turn it off", c.Server)
		}
		if len(c.Tools) > MaxMCPTools {
			return nil, invalid("scripts: at most %d tools per MCP server", MaxMCPTools)
		}
		tools := []string{}
		dup := map[string]bool{}
		for _, t := range c.Tools {
			if !mcpNameRE.MatchString(t) {
				return nil, invalid("scripts: MCP tool name %q is not valid", t)
			}
			if !dup[t] {
				dup[t] = true
				tools = append(tools, t)
			}
		}
		out = append(out, MCPChoice{Server: c.Server, Tools: tools})
	}
	return out, nil
}

// BuiltinEnv is the environment variable a built-in variable reaches a run through.
func BuiltinEnv(name string) string { return "KIRA_" + strings.ToUpper(name) }

// VarsUsed returns the built-in variable and param names that text uses in {...}, in first-seen order.
func VarsUsed(text string, params []Param) []string {
	known := map[string]bool{}
	for _, v := range BuiltinVars {
		known[v] = true
	}
	for _, p := range params {
		known[p.Name] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range varRE.FindAllStringSubmatch(text, -1) {
		if known[m[1]] && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

// Part is one piece of resolved text: literal Text, or a substituted variable (Var, Value).
type Part struct {
	Text  string `json:"text"`
	Var   string `json:"var"`
	Value string `json:"value"`
}

// Compose resolves text in one pass: a {name} with a value in vals becomes a Var part, anything
// else stays literal, and a value that contains a placeholder is not expanded again.
func Compose(text string, vals map[string]string) []Part {
	parts := []Part{}
	last := 0
	lit := func(to int) {
		if to > last {
			parts = append(parts, Part{Text: text[last:to]})
		}
	}
	for _, loc := range varRE.FindAllStringSubmatchIndex(text, -1) {
		name := text[loc[2]:loc[3]]
		v, ok := vals[name]
		if !ok {
			continue
		}
		lit(loc[0])
		parts = append(parts, Part{Var: name, Value: v})
		last = loc[1]
	}
	lit(len(text))
	return parts
}

// PlainText joins parts back into the text a run sends.
func PlainText(parts []Part) string {
	var sb strings.Builder
	for _, p := range parts {
		if p.Var != "" {
			sb.WriteString(p.Value)
		} else {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

// ParamValue is one resolved param.
type ParamValue struct {
	Name   string
	Secret bool
	// Rendered is the prompt form: text as is, a select's option, a multiselect joined by ", ".
	Rendered string
	// Env is the KIRA_PARAM_<NAME> value; a multiselect is newline-joined.
	Env string
}

// EnvName is the variable a param reaches the run through.
func EnvName(name string) string { return "KIRA_PARAM_" + strings.ToUpper(name) }

// ParamValues resolves the values given in the run dialog: defaults fill blanks, select values must
// be options, text is bounded. missing lists required params without a value.
func ParamValues(params []Param, given map[string][]string) (vals []ParamValue, missing []string, err error) {
	for name := range given {
		known := false
		for _, p := range params {
			known = known || p.Name == name
		}
		if !known {
			return nil, nil, fmt.Errorf("%q is not a param of this script", name)
		}
	}
	for _, p := range params {
		vs, err := resolveParam(p, given[p.Name])
		if err != nil {
			return nil, nil, err
		}
		if len(vs) == 0 && p.Required {
			missing = append(missing, p.Name)
		}
		vals = append(vals, ParamValue{Name: p.Name, Secret: p.Secret, Rendered: strings.Join(vs, ", "), Env: strings.Join(vs, "\n")})
	}
	return vals, missing, nil
}

// resolveParam applies the default to blank input and validates the values against the param.
func resolveParam(p Param, vs []string) ([]string, error) {
	if p.Type == ParamText {
		if len(vs) > 1 {
			return nil, fmt.Errorf("%s takes one value", p.Name)
		}
		if len(vs) == 1 && vs[0] == "" {
			vs = nil
		}
	}
	if len(vs) == 0 {
		vs = p.Default
	}
	if p.Type == ParamSelect && len(vs) > 1 {
		return nil, fmt.Errorf("%s takes one value", p.Name)
	}
	for _, v := range vs {
		if p.Type == ParamText {
			if err := checkText(v); err != nil {
				return nil, fmt.Errorf("%s %v", p.Name, err)
			}
		} else if !containsStr(p.Options, v) {
			return nil, fmt.Errorf("value %q is not an option of %s", v, p.Name)
		}
	}
	return vs, nil
}

// ToolPolicy is the script's tools in the provider-neutral vocabulary; extra are already-rendered
// names appended last.
func (s Smart) ToolPolicy(extra []string) harness.ToolPolicy {
	pol := harness.ToolPolicy{Builtins: s.Tools, BashPatterns: s.BashPatterns, Extra: extra}
	for _, c := range s.MCP {
		pol.MCP = append(pol.MCP, harness.MCPTools{Server: c.Server, AllTools: c.AllTools, Tools: c.Tools})
	}
	return pol
}
