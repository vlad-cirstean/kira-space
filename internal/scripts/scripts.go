// Package scripts is the one script (scripts) store and bound surface both
// apps share: the record type and its validation, the SQL repo, and the Wails service each app
// embeds under its own binding name (windowsvc's precedent).
package scripts

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/palette"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// A script's folder: the app's automations folder or a picked folder.
const (
	DirModeKira  = "kira"
	DirModeFixed = "fixed"
	// DirModeWorktree is a Dir.Mode only (a run from an ADE task); a script never stores it.
	DirModeWorktree = "worktree"
)

// MaxCollectionRunes caps a script collection name.
const MaxCollectionRunes = 64

// ValidationError marks a caller-input failure, answered as E_BAD_REQUEST. Path is a JSON pointer in
// the run file's vocabulary, Key a catalog key; no message echoes a value.
type ValidationError struct {
	Path string
	Key  string
	Args map[string]string
}

func (e *ValidationError) Error() string { return i18n.T(e.Key, e.Args) }

// invalid builds a ValidationError; kv are alternating argument names and values.
func invalid(path, key string, kv ...string) error {
	var args map[string]string
	if len(kv) > 0 {
		args = make(map[string]string, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			args[kv[i]] = kv[i+1]
		}
	}
	return &ValidationError{Path: path, Key: key, Args: args}
}

// IsCallerError reports whether err is the caller's fault: a validation failure or an unknown id.
func IsCallerError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve) || errors.Is(err, sql.ErrNoRows)
}

// CustomScript is one run, listed in the Automations panel: a projection of its run file plus the
// state columns. Mirrors packages/shared/domain/scripts.ts's customScriptSchema.
type CustomScript struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Command    string `json:"command"`
	WorkingDir string `json:"workingDir"`
	DirMode    string `json:"dirMode"`
	Color      string `json:"color"`
	// Collection groups the run in the panel; "" is ungrouped.
	Collection string `json:"collection"`
	SortOrder  int    `json:"sortOrder"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	// Kind is KindScript or KindSmart; for a smart script Command is the prompt.
	Kind   string  `json:"kind"`
	Params []Param `json:"params"`
	// Smart is nil for a normal script.
	Smart *Smart `json:"smart"`
	// UseAdeDir runs the script in the task's branch worktree when started from ADE. Only Kira
	// Space shows it.
	UseAdeDir bool `json:"useAdeDir"`
	// Schedule is nil for a script that is not recurring.
	Schedule *Schedule `json:"schedule"`
	// MCP is nil for a script without the mcp block (hidden from agents).
	MCP *MCPAccess `json:"mcp"`
	// SavedSecrets names the secret params with a saved value; never the value.
	SavedSecrets []string `json:"savedSecrets"`
}

// Collection is one panel group's order row; a collection exists while a run file names it.
type Collection struct {
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder"`
}

// Snapshot is the whole panel state: List answers it and every change broadcasts it.
type Snapshot struct {
	// Dir is the runs folder; the panel shows it.
	Dir         string         `json:"dir"`
	Collections []Collection   `json:"collections"`
	Scripts     []CustomScript `json:"scripts"`
	// Problems lists the files that are invalid now.
	Problems []FileProblem `json:"problems"`
	// SecretStorage is fixed for the process; saving a value is off when unavailable.
	SecretStorage secrets.Status `json:"secretStorage"`
}

// FileProblem is one invalid run file. ID is "" when the file name is not a valid id; LastGood says a
// previous valid version keeps running.
type FileProblem struct {
	File     string `json:"file"`
	ID       string `json:"id"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
	LastGood bool   `json:"lastGood"`
}

// CustomScriptFields is a run's config as Create accepts it and a file decodes to.
type CustomScriptFields struct {
	Name       string `json:"name"`
	Command    string `json:"command"`
	WorkingDir string `json:"workingDir"`
	// DirMode is one of the DirMode constants; "" means DirModeKira.
	DirMode string `json:"dirMode"`
	Color   string `json:"color"`
	// Collection groups the run in the panel; "" means ungrouped.
	Collection string `json:"collection"`
	// Kind is KindScript or KindSmart; "" means KindScript.
	Kind   string  `json:"kind"`
	Params []Param `json:"params"`
	// Smart is required for a smart script (missing fields take the defaults) and nil otherwise.
	Smart *Smart `json:"smart"`
	// UseAdeDir: see CustomScript.
	UseAdeDir bool `json:"useAdeDir"`
	// Schedule: see CustomScript.
	Schedule *Schedule `json:"schedule"`
	// MCP: see CustomScript.
	MCP *MCPAccess `json:"mcp"`
}

// Validate is the complete rule set, used for a Create and as the file rule. A pointer receiver:
// text is trimmed and defaults are filled in place. Errors carry a path in the file's vocabulary.
func (f *CustomScriptFields) Validate(models harness.Catalog) error {
	f.Name = strings.TrimSpace(f.Name)
	f.Command = strings.TrimSpace(f.Command)
	f.Collection = strings.TrimSpace(f.Collection)
	if f.Name == "" {
		return invalid("/name", "backend.core.runs.nameRequired.body")
	}
	if utf8.RuneCountInString(f.Collection) > MaxCollectionRunes {
		return invalid("/collection", "backend.core.runs.collectionLong.body", "max", strconv.Itoa(MaxCollectionRunes))
	}
	if f.Kind == "" {
		f.Kind = KindScript
	}
	switch f.Kind {
	case KindScript:
		if f.Smart != nil {
			return invalid("/smart", "backend.core.runs.scriptNoSmart.body")
		}
		if f.Command == "" {
			return invalid("/command", "backend.core.runs.commandRequired.body")
		}
	case KindSmart:
		if f.Command == "" {
			return invalid("/prompt", "backend.core.runs.promptRequired.body")
		}
		if utf8.RuneCountInString(f.Command) > MaxPrompt {
			return invalid("/prompt", "backend.core.scriptRun.promptTooLong.body", "max", strconv.Itoa(MaxPrompt))
		}
		smart, err := validSmart(f.Smart, models)
		if err != nil {
			return err
		}
		f.Smart = smart
	default:
		return invalid("/kind", "backend.core.runs.kindInvalid.body")
	}
	params, err := validParams(f.Params, f.Kind, f.Command)
	if err != nil {
		return err
	}
	f.Params = params
	if f.Schedule != nil {
		if err := validSchedule(f.Schedule, f.Kind, f.Params); err != nil {
			return err
		}
	}
	if f.MCP != nil {
		if err := validMCPAccess(f.MCP, f.Kind); err != nil {
			return err
		}
	}
	if f.DirMode == "" {
		f.DirMode = DirModeKira
	}
	switch f.DirMode {
	case DirModeKira:
		if f.WorkingDir != "" {
			return invalid("/path", "backend.core.runs.dirKiraPath.body")
		}
	case DirModeFixed:
		if !filepath.IsAbs(f.WorkingDir) {
			return invalid("/path", "backend.core.runs.dirFixedPath.body")
		}
	default:
		return invalid("/dir", "backend.core.runs.dirInvalid.body")
	}
	if f.Color == "" {
		f.Color = "none"
	}
	if !palette.Valid(f.Color) {
		return invalid("/color", "backend.core.runs.colorInvalid.body")
	}
	return nil
}
