package configsync

import (
	"regexp"
	"slices"
	"strings"
)

// Module names a sync can carry. repos syncs by origin URL, entry by entry: a shared repos file
// names its repository by origin, never by a local path, and only the shared keys of repocfg apply.
var syncModules = []string{"connections", "api", "runs", "workflows", "steps", "agent", "prompts", "repos"}

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

func isEntryModule(module string) bool {
	return module == "agent" || module == "prompts" || module == "repos"
}

func validMode(module, mode string) bool {
	switch mode {
	case ModeLocal:
		return slices.Contains(syncModules, module)
	case ModeShared:
		// A repos file merges into a local file of another name, so it is never taken whole.
		return slices.Contains(syncModules, module) && module != "repos"
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
	if module == "repos" {
		return []string{ModeLocal, ModeCombined}
	}
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

// isEntryFile: entries only for a combined settings or prompts file. repos plans its own entry files.
func isEntryFile(module, file, mode string) bool {
	return mode == ModeCombined && (module == "agent" && file == agentSettingsName || module == "prompts" && file == promptsName)
}
