package startupfail

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// detailCap bounds Message.Detail: the alert body carries a collapsed, single-line, truncated
// summary of the raw error; the *Copy Details* clipboard payload (render.go's RenderClipboard)
// carries the full, uncapped error instead, so nothing is ever lost — only abbreviated in the part
// macOS renders on screen.
const detailCap = 500

// Message is Classify's whole output — everything render.go needs to compose the alert title/body
// and the clipboard payload, and the posture (Expected) both the message and the presenter honour.
type Message struct {
	Step Step
	// Expected reports whether this is a state the user can act on (a full disk, a downgraded
	// app, a locked database) as opposed to a defect in Kira Studio itself. Expected: false never
	// pretends to be actionable — its Advice always says, honestly, that this is a bug.
	Expected bool
	// Headline is the alert's title (item 1 of argv, D3) — one short, plain sentence. It NEVER
	// contains the raw error text (classify_test.go asserts this for every step).
	Headline string
	// Advice is one or two sentences telling the user what, if anything, they can do about it.
	Advice string
	// Detail is the raw error, collapsed to a single line and capped at detailCap bytes.
	Detail string
}

// schemaTooNew is the duck-typed interface through which internal/storage's SchemaTooNewError is
// recognised, without importing internal/storage at all: this package's whole job is formatting
// strings and building an argv, and pulling in modernc.org/sqlite just so it could use errors.As
// on the real type would make it something other than a true leaf. Any error implementing this
// method — including internal/gitreview's byte-identical refusal, if a future phase ever needs it
// to — is recognised the same way.
type schemaTooNew interface {
	SchemaTooNew() (found, known int)
}

// schemaTooOld is the duck-typed interface for sqlitex.SchemaTooOldError, for the same reason.
type schemaTooOld interface {
	SchemaTooOld() (found, baseline int)
}

// collapse turns a possibly multi-line, possibly huge error string into one line bounded to max
// bytes, so what a child process said never reaches a UI string unbounded. Every newline is
// collapsed, not only the first, since a Go error's own text can carry embedded newlines anywhere.
func collapse(s string, max int) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	if len(s) > max {
		for max > 0 && !utf8.RuneStart(s[max]) {
			max--
		}
		s = s[:max]
	}
	return s
}

// bugAdvice is the honest, non-actionable advice every Expected: false step gives — never a fake
// remedy for a state that is, in truth, a defect in Kira Studio itself.
const bugAdvice = "This is a bug. Please report it with the details below."

func bugMessage(step Step, headline, detail string) Message {
	return Message{Step: step, Expected: false, Headline: headline, Advice: bugAdvice, Detail: detail}
}

// Classify implements the table exactly: one Message shape for every Step, plus the
// schema-too-new variant of StepStorage recognised via the schemaTooNew interface above —
// deliberately not a tenth Step constant (the note: it is "a variant of StepStorage not a
// separate Step").
//
// The switch below carries no default case: Go's switch gives no compile-time exhaustiveness
// guarantee over a defined string type on its own, so classify_test.go's table test — which
// enumerates every Step constant declared in step.go — is what catches a Step added
// without a matching arm here; the one line after the switch is a safe, honest fallback for a Step
// value nothing in this codebase ever constructs, not a substitute for that test.
func Classify(step Step, err error, info Info) Message {
	detail := ""
	if err != nil {
		detail = collapse(err.Error(), detailCap)
	}
	app := info.AppName

	switch step {
	case StepEnsureLayout:
		return Message{
			Step: step, Expected: true,
			Headline: app + " couldn't create its data folder.",
			Advice: "Check that " + info.home() +
				" exists, is a folder you can write to, and that the disk isn't full.",
			Detail: detail,
		}
	case StepLogging:
		return Message{
			Step: step, Expected: true,
			Headline: app + " couldn't open its log folder.",
			Advice: "Check that " + info.logsDir() +
				" exists, is a folder you can write to, and that the disk isn't full.",
			Detail: detail,
		}
	case StepStorage:
		if tn, ok := err.(schemaTooNew); ok {
			found, known := tn.SchemaTooNew()
			return Message{
				Step: step, Expected: true,
				Headline: "This copy of " + app + " is older than your data.",
				Advice: fmt.Sprintf(
					"Your %s data is at format version %d; this copy (%s) understands up "+
						"to %d. Open the newer version of %s, or update this one. Your "+
						"data has not been changed.",
					app, found, info.Version, known, app,
				),
				Detail: detail,
			}
		}
		if to, ok := err.(schemaTooOld); ok {
			found, baseline := to.SchemaTooOld()
			return Message{
				Step: step, Expected: true,
				Headline: "Your " + app + " data predates v0.3.1.",
				Advice: fmt.Sprintf(
					"Your data is at format version %d; this copy (%s) starts at %d. "+
						"Open it with v0.3.1 first, then update. Your data has not been changed.",
					found, info.Version, baseline,
				),
				Detail: detail,
			}
		}
		return Message{
			Step: step, Expected: true,
			Headline: app + " couldn't open its database.",
			Advice: "The database at " + info.dbPath() +
				" may be damaged, unreadable, or in use by another copy of " + app + ". Quit any " +
				"other copy and try again.",
			Detail: detail,
		}
	case StepRepos:
		return bugMessage(step, app+" couldn't prepare its database.", detail)
	case StepSettings:
		return bugMessage(step, app+" couldn't read its settings.", detail)
	case StepWindowList:
		return bugMessage(step, app+" couldn't read its saved windows.", detail)
	case StepWindowCreate:
		return Message{
			Step: step, Expected: true,
			Headline: app + " couldn't save its window layout.",
			Advice: "The disk may be full, or the database at " + info.dbPath() +
				" may be read-only.",
			Detail: detail,
		}
	case StepInstanceLock:
		return Message{
			Step: step, Expected: true,
			Headline: app + " couldn't check for another running copy.",
			Advice: "Check that " + info.home() +
				" exists, is a folder you can write to, and that the disk isn't full.",
			Detail: detail,
		}
	case StepRun:
		return bugMessage(step, app+" couldn't open its main window.", detail)
	case StepPlatform:
		return bugMessage(step, app+" couldn't start.", detail)
	}
	return bugMessage(step, app+" couldn't start.", detail)
}
