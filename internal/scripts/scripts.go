// Package scripts is the one script (custom_scripts) store and bound surface both
// apps share: the record type and its validation, the SQL repo, and the Wails service each app
// embeds under its own binding name (windowsvc's precedent).
package scripts

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/palette"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// A script's folder: the app's automations folder, a picked folder, or (legacy rows only) $HOME.
const (
	DirModeKira  = "kira"
	DirModeFixed = "fixed"
	DirModeHome  = "home"
	// DirModeWorktree is a Dir.Mode only (a run from an ADE task); a script never stores it.
	DirModeWorktree = "worktree"
)

// MaxCollectionRunes caps a script collection name.
const MaxCollectionRunes = 64

// ValidationError marks a caller-input failure, answered as E_BAD_REQUEST.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// IsCallerError reports whether err is the caller's fault: a validation failure or an unknown id.
func IsCallerError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve) || errors.Is(err, sql.ErrNoRows)
}

// CustomScript mirrors packages/shared/domain/scripts.ts's customScriptSchema: one custom_scripts
// row, listed in the Automations panel.
type CustomScript struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Command    string `json:"command"`
	WorkingDir string `json:"workingDir"`
	DirMode    string `json:"dirMode"`
	Color      string `json:"color"`
	// CollectionID is nil for an ungrouped script.
	CollectionID *string `json:"collectionId"`
	SortOrder    int     `json:"sortOrder"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
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
	// SavedSecrets names the secret params with a saved value; never the value.
	SavedSecrets []string `json:"savedSecrets"`
}

// Collection mirrors packages/shared/domain/scripts.ts's scriptCollectionSchema: one
// custom_script_collections row, the group a script lives under.
type Collection struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Snapshot is the whole panel state: List answers it and every mutation broadcasts it.
type Snapshot struct {
	Collections []Collection   `json:"collections"`
	Scripts     []CustomScript `json:"scripts"`
	// SecretStorage is fixed for the process; the editor disables saving a value when off.
	SecretStorage secrets.Status `json:"secretStorage"`
}

// CustomScriptFields is what Create/Update accept (customScriptFieldsSchema).
type CustomScriptFields struct {
	Name       string `json:"name"`
	Command    string `json:"command"`
	WorkingDir string `json:"workingDir"`
	// DirMode is one of the DirMode constants; "" means DirModeKira.
	DirMode string `json:"dirMode"`
	Color   string `json:"color"`
	// CollectionID groups the command in the panel; nil means ungrouped.
	CollectionID *string `json:"collectionId"`
	// Kind is KindScript or KindSmart; "" means KindScript.
	Kind   string  `json:"kind"`
	Params []Param `json:"params"`
	// Smart is required for a smart script (missing fields take the defaults) and nil otherwise.
	Smart *Smart `json:"smart"`
	// UseAdeDir: see CustomScript.
	UseAdeDir bool `json:"useAdeDir"`
	// Schedule: see CustomScript.
	Schedule *Schedule `json:"schedule"`
}

// Validate is the complete rule set: the Go check is the authority, the mirrored zod schema is
// only the dialog's affordance. A pointer receiver: name and command are trimmed in
// place (leading/trailing only) so Create/Update persist the trimmed values.
func (f *CustomScriptFields) Validate(models harness.Catalog) error {
	f.Name = strings.TrimSpace(f.Name)
	f.Command = strings.TrimSpace(f.Command)
	if f.Name == "" {
		return invalid("scripts: name is required")
	}
	if f.Kind == "" {
		f.Kind = KindScript
	}
	switch f.Kind {
	case KindScript:
		if f.Smart != nil {
			return invalid("scripts: only a smart script has smart settings")
		}
		if f.Command == "" {
			return invalid("scripts: command is required")
		}
	case KindSmart:
		if f.Command == "" {
			return invalid("scripts: prompt is required")
		}
		if utf8.RuneCountInString(f.Command) > MaxPrompt {
			return invalid("scripts: prompt is longer than %d characters", MaxPrompt)
		}
		smart, err := validSmart(f.Smart, models)
		if err != nil {
			return err
		}
		f.Smart = smart
	default:
		return invalid("scripts: kind must be script or smart")
	}
	params, err := validParams(f.Params, f.Kind, f.Command)
	if err != nil {
		return err
	}
	f.Params = params
	if f.Schedule != nil {
		if err := validSchedule(f.Schedule, f.Kind, f.Command, f.Params); err != nil {
			return err
		}
	}
	if f.DirMode == "" {
		f.DirMode = DirModeKira
	}
	switch f.DirMode {
	case DirModeKira, DirModeHome:
		f.WorkingDir = ""
	case DirModeFixed:
		if !filepath.IsAbs(f.WorkingDir) {
			return invalid("scripts: working directory must be an absolute path")
		}
	default:
		return invalid("scripts: invalid directory mode")
	}
	if !palette.Valid(f.Color) {
		return invalid("scripts: invalid colour")
	}
	return nil
}

// validCollectionName trims name and enforces non-empty and MaxCollectionRunes.
func validCollectionName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", invalid("scripts: collection name is required")
	}
	if utf8.RuneCountInString(name) > MaxCollectionRunes {
		return "", invalid("scripts: collection name is too long")
	}
	return name, nil
}

// errHomeRetired refuses the legacy home folder for a new choice.
var errHomeRetired = invalid("scripts: the home folder is no longer offered")
