package scripts

import (
	"fmt"
	"regexp"
	"strconv"
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
	Model        string  `json:"model"`
	MaxBudgetUSD float64 `json:"maxBudgetUsd"`
	// NoCostCap runs with no cost cap and stores MaxBudgetUSD 0, since a stored 0 otherwise means
	// the default. A provider that cannot cap cost (OpenCode) runs only such a script.
	NoCostCap    bool        `json:"noCostCap"`
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
		return nil, invalid("/params", "backend.core.runs.paramsMax.body", "max", strconv.Itoa(MaxParams))
	}
	out := make([]Param, 0, len(params))
	seen := map[string]bool{}
	for i, p := range params {
		at := "/params/" + strconv.Itoa(i)
		p.Name = strings.TrimSpace(p.Name)
		switch {
		case !paramNameRE.MatchString(p.Name):
			return nil, invalid(at+"/name", "backend.core.runs.paramName.body")
		case containsStr(BuiltinVars, p.Name):
			return nil, invalid(at+"/name", "backend.core.runs.paramReserved.body")
		case seen[p.Name]:
			return nil, invalid(at+"/name", "backend.core.runs.paramDuplicate.body")
		}
		seen[p.Name] = true
		var err error
		if p, err = validParamShape(p, at); err != nil {
			return nil, err
		}
		if p.Secret && kind == KindSmart && containsVar(prompt, p.Name) {
			return nil, invalid("/prompt", "backend.core.runs.paramSecretInPrompt.body")
		}
		out = append(out, p)
	}
	return out, nil
}

func validParamShape(p Param, at string) (Param, error) {
	p.Options = nonNil(p.Options)
	p.Default = nonNil(p.Default)
	switch p.Type {
	case ParamText:
		return p, validTextParam(p, at)
	case ParamSelect, ParamMultiselect:
		return p, validChoiceParam(p, at)
	}
	return p, invalid(at+"/type", "backend.core.runs.paramType.body")
}

func validTextParam(p Param, at string) error {
	if len(p.Options) > 0 {
		return invalid(at+"/options", "backend.core.runs.paramTextOptions.body")
	}
	if len(p.Default) > 1 {
		return invalid(at+"/default", "backend.core.runs.paramTextDefault.body")
	}
	if len(p.Default) == 1 {
		if p.Secret {
			return invalid(at+"/default", "backend.core.runs.paramSecretDefault.body")
		}
		if key := textProblem(p.Default[0]); key != "" {
			return invalid(at+"/default", key, "max", strconv.Itoa(MaxTextValue))
		}
	}
	return nil
}

// validChoiceParam checks a select or multiselect param; it trims p.Options in place.
func validChoiceParam(p Param, at string) error {
	if p.Secret {
		return invalid(at+"/secret", "backend.core.runs.paramChoiceSecret.body")
	}
	if len(p.Options) == 0 || len(p.Options) > MaxOptions {
		return invalid(at+"/options", "backend.core.runs.paramOptionsCount.body", "max", strconv.Itoa(MaxOptions))
	}
	seen := map[string]bool{}
	for i, o := range p.Options {
		o = strings.TrimSpace(o)
		p.Options[i] = o
		opt := at + "/options/" + strconv.Itoa(i)
		switch {
		case o == "" || utf8.RuneCountInString(o) > MaxOptionRunes || strings.ContainsAny(o, "\r\n"):
			return invalid(opt, "backend.core.runs.paramOptionInvalid.body", "max", strconv.Itoa(MaxOptionRunes))
		case seen[o]:
			return invalid(opt, "backend.core.runs.paramOptionDuplicate.body")
		}
		seen[o] = true
	}
	if p.Type == ParamSelect && len(p.Default) > 1 {
		return invalid(at+"/default", "backend.core.runs.paramSelectDefault.body")
	}
	for i, d := range p.Default {
		if !containsStr(p.Options, d) {
			return invalid(at+"/default/"+strconv.Itoa(i), "backend.core.runs.paramDefaultNotOption.body")
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

// textProblem returns the catalog key of what is wrong with a text value, "" when it is fine.
func textProblem(v string) string {
	if utf8.RuneCountInString(v) > MaxTextValue {
		return "backend.core.runs.textTooLong.body"
	}
	for _, r := range v {
		if r == 0 || (unicode.IsControl(r) && r != '\n' && r != '\t') {
			return "backend.core.runs.textControl.body"
		}
	}
	return ""
}

// checkText bounds a text value: no NUL or control characters except newline and tab.
func checkText(v string) error {
	if key := textProblem(v); key != "" {
		return invalid("", key, "max", strconv.Itoa(MaxTextValue))
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
		return nil, invalid("/smart/model", "backend.core.runs.smartModel.body", "allowed", orList(models.Aliases))
	}
	switch {
	case out.NoCostCap:
		out.MaxBudgetUSD = 0
	case out.MaxBudgetUSD == 0:
		out.MaxBudgetUSD = DefaultBudgetUSD
	case out.MaxBudgetUSD < MinBudgetUSD || out.MaxBudgetUSD > MaxBudgetUSD:
		return nil, invalid("/smart/max_budget_usd", "backend.core.runs.smartBudget.body",
			"min", fmt.Sprintf("%.2f", MinBudgetUSD), "max", fmt.Sprintf("%.0f", MaxBudgetUSD))
	}
	out.Timeout = strings.TrimSpace(out.Timeout)
	if out.Timeout == "" {
		out.Timeout = DefaultTimeout
	}
	d, err := time.ParseDuration(out.Timeout)
	if err != nil || d < MinTimeout || d > MaxTimeout {
		return nil, invalid("/smart/timeout", "backend.core.runs.smartTimeout.body")
	}
	if out.Tools == nil {
		out.Tools = append([]string{}, DefaultTools...)
	}
	picked := map[string]bool{}
	for i, t := range out.Tools {
		if !containsStr(BuiltinTools, t) {
			return nil, invalid("/smart/tools/"+strconv.Itoa(i), "backend.core.runs.smartTool.body")
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
		return nil, invalid("/smart/bash_patterns", "backend.core.runs.smartPatternsNeedBash.body")
	}
	if len(patterns) > MaxPatterns {
		return nil, invalid("/smart/bash_patterns", "backend.core.runs.smartPatternsMax.body", "max", strconv.Itoa(MaxPatterns))
	}
	seen := map[string]bool{}
	for i, p := range patterns {
		at := "/smart/bash_patterns/" + strconv.Itoa(i)
		p = strings.TrimSpace(p)
		switch {
		case p == "" || utf8.RuneCountInString(p) > MaxPatternRunes:
			return nil, invalid(at, "backend.core.runs.smartPatternLength.body", "max", strconv.Itoa(MaxPatternRunes))
		case strings.ContainsAny(p, ")\r\n"):
			return nil, invalid(at, "backend.core.runs.smartPatternChars.body")
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
		return nil, invalid("/smart/mcp", "backend.core.runs.smartMcpMax.body", "max", strconv.Itoa(MaxMCPServers))
	}
	seen := map[string]bool{}
	for i, c := range choices {
		at := "/smart/mcp/" + strconv.Itoa(i)
		if !mcpNameRE.MatchString(c.Server) {
			return nil, invalid(at+"/server", "backend.core.runs.smartMcpName.body")
		}
		if seen[c.Server] {
			return nil, invalid(at+"/server", "backend.core.runs.smartMcpDuplicate.body")
		}
		seen[c.Server] = true
		if c.AllTools {
			out = append(out, MCPChoice{Server: c.Server, AllTools: true, Tools: []string{}})
			continue
		}
		if len(c.Tools) == 0 {
			return nil, invalid(at+"/tools", "backend.core.runs.smartMcpNeedTool.body")
		}
		if len(c.Tools) > MaxMCPTools {
			return nil, invalid(at+"/tools", "backend.core.runs.smartMcpToolsMax.body", "max", strconv.Itoa(MaxMCPTools))
		}
		tools := []string{}
		dup := map[string]bool{}
		for j, t := range c.Tools {
			if !mcpNameRE.MatchString(t) {
				return nil, invalid(at+"/tools/"+strconv.Itoa(j), "backend.core.runs.smartMcpToolName.body")
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
