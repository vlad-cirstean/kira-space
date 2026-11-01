package adeflow

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

//go:embed agents_intro.md
var agentsIntro string

// rule documents one cross-field check of the validator. rules_test.go proves each one still rejects its example.
type rule struct{ ID, Doc string }

var rules = []rule{
	{"stem", "The file name without `.yaml` equals `id`."},
	{"stages", "`stages` is no longer supported. List every step under `steps`."},
	{"steps", "`steps` is not empty and at least one step is not skipped."},
	{"unique-step", "Step ids are unique."},
	{"kind-keys", "A step takes only the keys of its kind (see each option)."},
	{"agent-body", "An agent step has either `prompt` (with optional `allowed_tools`) or `smart_script` (with optional `params`), never both."},
	{"manual-session", "A manual step takes `prompt` and `settings` only with `session: true`."},
	{"script-results", "A script step declares no `results`, or exactly `done` (ok: true) then `failed` (ok: false)."},
	{"unique-result", "Result ids are unique in a step. `needs_input` is reserved."},
	{"routes", "`next` is `next`, `end`, `stop` or a step id. `stop` is only for a result with `ok: false`."},
	{"returns", "A `next` to the same or an earlier step is a return, counted against `max_returns`. It never targets a manual or skipped step."},
	{"forward", "At least one result of a step leads forward."},
	{"implementer", "`review.implementer` is an earlier agent step without `smart_script`."},
	{"sandbox-off", "`sandbox` with `access: off` takes no other key."},
	{"sandbox-paths", "Sandbox paths are absolute or start with `~/`. They never name the root, the home folder or an ancestor of it, an agent config folder or the keychain folder."},
	{"legacy-status", "The old status names `To do`, `In progress`, `In review` and `Done` read as their ids."},
	{"runtime-checks", "Whether a `status` exists and whether an `only <repo>` repo exists is checked when a task runs, not when the file loads."},
}

// DocModel is the source of the workflows folder's AGENTS.md, built from the option table, the
// agent catalog and the rule list.
func DocModel() configfs.DocModel {
	m := configfs.DocModel{
		App:     "Space",
		Title:   "Workflows",
		Intro:   agentsIntro,
		Pattern: "<id>.yaml",
		Format:  "YAML",
	}
	for _, r := range rules {
		m.Rules = append(m.Rules, r.Doc)
	}
	for _, o := range options {
		m.Options = append(m.Options, configfs.DocOption{
			Path:        o.Path,
			Type:        o.Type,
			Required:    o.Required,
			Default:     o.Default,
			Enum:        o.Enum,
			Range:       o.Range,
			Description: optionDoc(o),
		})
		if o.Path == "steps[].settings" {
			m.Options = append(m.Options, settingOptions()...)
		}
	}
	return m
}

// optionDoc puts the facts derived from the table's kind and flag fields in front of its line.
func optionDoc(o option) string {
	var parts []string
	if o.Legacy {
		parts = append(parts, "Legacy: accepted and ignored; Kira drops it on save.")
	}
	if o.Kinds != nil {
		k := kindList(o.Kinds)
		parts = append(parts, strings.ToUpper(k[:1])+k[1:]+" only.")
	}
	if len(o.RequiredOn) > 0 {
		parts = append(parts, "Required on "+kindList(o.RequiredOn)+".")
	}
	return strings.Join(append(parts, o.Doc), " ")
}

// kindList renders kinds as "agent, script and review steps".
func kindList(ks []adewire.StepKind) string {
	return strings.Replace(oneOf(ks), " or ", " and ", 1) + " steps"
}

// settingOptions lists the per-agent settings from the stage catalog, so a new agent shows up with no change here.
func settingOptions() []configfs.DocOption {
	var out []configfs.DocOption
	for _, set := range stageCatalog {
		out = append(out, configfs.DocOption{
			Path: "steps[].settings." + set.ID,
			Type: "mapping",
		})
		for _, s := range set.Settings {
			args := map[string]string{"min": strconv.Itoa(s.Min), "max": strconv.Itoa(s.Max)}
			out = append(out, configfs.DocOption{
				Path:        fmt.Sprintf("steps[].settings.%s.%s", set.ID, s.Key),
				Type:        "whole number",
				Range:       fmt.Sprintf("minimum %d, maximum %d", s.Min, s.Max),
				Description: i18n.T(s.LabelKey, nil) + ". " + i18n.T(s.HintKey, args),
			})
		}
	}
	return out
}
