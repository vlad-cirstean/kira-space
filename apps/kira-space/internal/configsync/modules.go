package configsync

import (
	"regexp"
	"slices"
	"strings"
)

// Module names a sync can carry. repos is never one: its files are keyed by local path and carry
// the worktree prepare script, which never comes from a repo-carried source.
var syncModules = []string{"connections", "api", "runs", "workflows", "steps", "agent", "prompts"}

// Modes of a module.
const (
	ModeLocal    = "local"
	ModeShared   = "shared"
	ModeCombined = "combined"
)

const (
	agentSettingsName = "settings.yaml"
	agentContextName  = "context.md"
	promptsName       = "llm-prompts.json"
	manifestName      = "kira-config.yaml"
)

func isEntryModule(module string) bool { return module == "agent" || module == "prompts" }

func validMode(module, mode string) bool {
	switch mode {
	case ModeLocal, ModeShared:
		return slices.Contains(syncModules, module)
	case ModeCombined:
		return isEntryModule(module)
	}
	return false
}

func defaultMode(module string) string {
	if isEntryModule(module) {
		return ModeCombined
	}
	return ModeShared
}

// modesOf lists the modes a module offers.
func modesOf(module string) []string {
	if isEntryModule(module) {
		return []string{ModeLocal, ModeShared, ModeCombined}
	}
	return []string{ModeLocal, ModeShared}
}

// runsCommands marks modules whose content starts processes.
func runsCommands(module string) bool {
	return module == "runs" || module == "steps" || module == "workflows"
}

var fileNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// syncableFile reports whether a shared file of module is read: hidden files, the generated docs
// file and per-machine *.local.yaml never are.
func syncableFile(module, name string) bool {
	if !fileNameRe.MatchString(name) || name == "AGENTS.md" || strings.HasSuffix(name, ".local.yaml") {
		return false
	}
	switch module {
	case "agent":
		return name == agentSettingsName || name == agentContextName
	case "prompts":
		return name == promptsName
	}
	return strings.HasSuffix(name, ".yaml")
}

// unitKind of a file under a mode: entries only for a combined settings or prompts file.
func isEntryFile(module, file, mode string) bool {
	return mode == ModeCombined && (module == "agent" && file == agentSettingsName || module == "prompts" && file == promptsName)
}
