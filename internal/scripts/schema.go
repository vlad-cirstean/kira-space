package scripts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/palette"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

//go:embed run.schema.json
var baseSchema []byte

//go:embed runs_intro.md
var runsIntro string

// assembleSchema writes every enum and bound the Go validator owns into the schema, so the schema
// and AGENTS.md cannot drift from the code. spaceKeys false drops the Space-only use_ade_dir.
func assembleSchema(models harness.Catalog, spaceKeys bool) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(baseSchema, &root); err != nil {
		return nil, fmt.Errorf("scripts: schema: %w", err)
	}
	props := root["properties"].(map[string]any)
	sub := func(m map[string]any, keys ...string) map[string]any {
		for _, k := range keys {
			m = m[k].(map[string]any)
		}
		return m
	}
	if !spaceKeys {
		delete(props, "use_ade_dir")
	}
	strs := func(in []string) []any {
		out := make([]any, len(in))
		for i, s := range in {
			out[i] = s
		}
		return out
	}
	sub(props, "color")["enum"] = strs(palette.Names())
	sub(props, "collection")["maxLength"] = MaxCollectionRunes
	sub(props, "prompt")["maxLength"] = MaxPrompt
	params := sub(props, "params")
	params["maxItems"] = MaxParams
	pp := sub(params, "items", "properties")
	sub(pp, "options")["maxItems"] = MaxOptions
	sub(pp, "options", "items")["maxLength"] = MaxOptionRunes
	smart := sub(props, "smart", "properties")
	sub(smart, "model")["enum"] = strs(models.Aliases)
	sub(smart, "max_budget_usd")["maximum"] = MaxBudgetUSD
	sub(smart, "tools", "items")["enum"] = strs(BuiltinTools)
	sub(smart, "bash_patterns")["maxItems"] = MaxPatterns
	sub(smart, "bash_patterns", "items")["maxLength"] = MaxPatternRunes
	mcp := sub(smart, "mcp")
	mcp["maxItems"] = MaxMCPServers
	sub(mcp, "items", "properties", "tools")["maxItems"] = MaxMCPTools
	return json.Marshal(root)
}

// ruleOwners lists, per documented rule, the message keys it can raise. Validate is one check; each
// rule reports only the failures of its own area, so every rule has its doc and its check.
type ruleOwner struct {
	doc  string
	owns func(key string) bool
}

func keyIn(keys ...string) func(string) bool {
	set := map[string]bool{}
	for _, k := range keys {
		set["backend.core.runs."+k+".body"] = true
	}
	return func(k string) bool { return set[k] }
}

func keyPrefix(prefix string) func(string) bool {
	return func(k string) bool { return strings.HasPrefix(k, "backend.core.runs."+prefix) }
}

func ruleOwners() []ruleOwner {
	return []ruleOwner{
		{"A param name is unique and is not a built-in variable (task, jira, repo, branch, base, worktree).",
			keyIn("paramsMax", "paramName", "paramReserved", "paramDuplicate", "paramType")},
		{"A text param has no options and at most one default. A secret param has no default.",
			keyIn("paramTextOptions", "paramTextDefault", "paramSecretDefault", "textTooLong", "textControl")},
		{"A select or multiselect param has options, a default only among them, and a select has at most one default. Only a text param can be secret.",
			keyIn("paramChoiceSecret", "paramOptionsCount", "paramOptionInvalid", "paramOptionDuplicate", "paramSelectDefault", "paramDefaultNotOption")},
		{"A secret param never appears in the prompt.", keyIn("paramSecretInPrompt")},
		{"dir fixed needs an absolute path. dir kira has no path.", keyIn("dirKiraPath", "dirFixedPath", "dirInvalid")},
		{"A smart run keeps its budget between 0.05 and 25 USD unless no_cost_cap is on, its timeout between 1m and 2h, and bash_patterns only with the Bash tool. MCP servers are unique and each lists tools or sets all_tools.",
			keyPrefix("smart")},
		{"A schedule has a valid 5-field cron and a known timezone. A script's timeout is between 1m and 24h and a smart run has none. params name non-secret params with values they accept.",
			func(k string) bool {
				return keyPrefix("cron")(k) || keyPrefix("timezone")(k) || keyPrefix("schedule")(k)
			}},
	}
}

const kindRuleDoc = "kind script needs command and has no prompt and no smart. kind smart needs prompt and has no command. name is not empty."

// fileRules builds the configfs rules. The first owns every failure no other rule claims.
func fileRules(models harness.Catalog) []configfs.Rule[File] {
	owners := ruleOwners()
	rules := make([]configfs.Rule[File], 0, len(owners)+1)
	rules = append(rules, configfs.Rule[File]{Doc: kindRuleDoc, Check: func(f File, _ configfs.Locator) *configfs.Problem {
		switch {
		case f.Kind != KindSmart && f.Prompt != "":
			return &configfs.Problem{Path: "/prompt", Message: i18n.T("backend.core.runs.scriptNoPrompt.body", nil)}
		case f.Kind == KindSmart && f.Command != "":
			return &configfs.Problem{Path: "/command", Message: i18n.T("backend.core.runs.smartNoCommand.body", nil)}
		}
		err := checkFile(f, models)
		if err == nil {
			return nil
		}
		for _, o := range owners {
			if o.owns(err.Key) {
				return nil
			}
		}
		return problemOf(err)
	}})
	for _, o := range owners {
		rules = append(rules, configfs.Rule[File]{Doc: o.doc, Check: func(f File, _ configfs.Locator) *configfs.Problem {
			if err := checkFile(f, models); err != nil && o.owns(err.Key) {
				return problemOf(err)
			}
			return nil
		}})
	}
	return rules
}

// checkFile runs the one validator on a file's fields.
func checkFile(f File, models harness.Catalog) *ValidationError {
	fields := f.Fields()
	if f.Kind != KindSmart && f.Prompt != "" || f.Kind == KindSmart && f.Command != "" {
		return nil
	}
	err := fields.Validate(models)
	if err == nil {
		return nil
	}
	ve, _ := err.(*ValidationError)
	return ve
}

func problemOf(e *ValidationError) *configfs.Problem {
	return &configfs.Problem{Path: e.Path, Message: e.Error()}
}

const stemRuleDoc = "The file name without .yaml is the run id: lowercase letters, digits, - and _, starting with a letter or digit, up to 80 characters. A file with another name is listed but not used."

// newDecoder builds the file decoder and its AGENTS.md model.
func newDecoder(models harness.Catalog, spaceKeys bool, app string) (configfs.DecodeFunc[File], configfs.DocModel, error) {
	schema, err := assembleSchema(models, spaceKeys)
	if err != nil {
		return nil, configfs.DocModel{}, err
	}
	decode, model, err := configfs.YAMLSchema[File](schema, fileRules(models)...)
	if err != nil {
		return nil, configfs.DocModel{}, err
	}
	model.Rules = append(model.Rules, stemRuleDoc)
	model.App, model.Title, model.Intro, model.Pattern, model.Format = app, "Runs", runsIntro, "<id>.yaml", "YAML"
	return func(file string, src []byte) (File, *configfs.Problem) {
		if _, ok := StemOf(file); !ok {
			return File{}, &configfs.Problem{Message: i18n.T("backend.core.runs.stemInvalid.body", nil)}
		}
		return decode(file, src)
	}, model, nil
}
