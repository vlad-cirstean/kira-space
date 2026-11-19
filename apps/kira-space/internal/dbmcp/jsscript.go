package dbmcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/jsengine"
)

// ScriptEngine is dbmcp's consumer-declared view of *jsengine.Engine.
type ScriptEngine interface {
	Exec(ctx context.Context, r jsengine.Run) (jsengine.Result, error)
}

// OpRunner records an operation row (*adapterhost.Host satisfies it).
type OpRunner interface {
	RunOp(ctx context.Context, spec adapterhost.OpSpec, fn func(context.Context, *adapters.OpCtx) (any, error)) (string, any, error)
}

type scriptStage string

const (
	stagePrepare   scriptStage = "prepare"
	stageTransform scriptStage = "transform"
)

// opKind is the op log kind of the stage's row.
func (st scriptStage) opKind() string {
	if st == stagePrepare {
		return "jsPrepare"
	}
	return "jsTransform"
}

// scriptRun is one engine run. Bindings is built by appending: later phases add theirs before execScript.
type scriptRun struct {
	Stage    scriptStage
	Source   string
	Bindings []jsengine.Binding
	Limits   jsengine.Limits
	// ConnectionID ties the op row to a connection (run_query scripts); empty for none.
	ConnectionID string
	// Validate checks a successful result; a non-empty text fails the run (and its op row) with that text.
	Validate func(jsengine.Result) string
}

// scriptsOn reports whether scripts can run: the user's switch plus an engine and an op recorder.
func (s *Server) scriptsOn() bool {
	return s.cfg.JS != nil && s.cfg.ScriptOps != nil && s.apiSettings().Scripts
}

// scriptErrText is the engine's message; an internal fault gets a static one.
func scriptErrText(e *jsengine.Error) string {
	if e.Kind == jsengine.KindInternal {
		return "internal error"
	}
	return e.Error()
}

// checkScript syntax-checks src. A failure records the stage's op row and returns the agent text; "" is ok.
// label names the script in the text and nothing says what did not happen ("Nothing was sent.").
func (s *Server) checkScript(ctx context.Context, st scriptStage, src, label, nothing string) string {
	e := jsengine.Check(src)
	if e == nil {
		return ""
	}
	text := fmt.Sprintf("%s: %s. %s", label, scriptErrText(e), nothing)
	_, _, _ = s.cfg.ScriptOps.RunOp(ctx, adapterhost.OpSpec{Kind: st.opKind()}, func(_ context.Context, op *adapters.OpCtx) (any, error) {
		op.SetCommand(visibleScript(src))
		return nil, errors.New(text)
	})
	return text
}

// execScript runs r under one op row whose command is the source and whose error is the returned
// failure text, so the agent and the op log show the same words. describe builds that text per stage.
func (s *Server) execScript(ctx context.Context, r scriptRun, describe func(*jsengine.Error) string) (jsengine.Result, string) {
	var res jsengine.Result
	failure := ""
	spec := adapterhost.OpSpec{Kind: r.Stage.opKind()}
	if r.ConnectionID != "" {
		spec.ConnectionID = &r.ConnectionID
	}
	_, _, err := s.cfg.ScriptOps.RunOp(ctx, spec, func(runCtx context.Context, op *adapters.OpCtx) (any, error) {
		op.SetCommand(r.Source)
		out, err := s.cfg.JS.Exec(runCtx, jsengine.Run{Source: r.Source, Bindings: r.Bindings, Limits: r.Limits})
		if err != nil {
			var je *jsengine.Error
			if !errors.As(err, &je) {
				s.log.Warn("dbmcp: script run failed", "scope", "dbmcp", "stage", string(r.Stage), "err", err)
				je = &jsengine.Error{Kind: jsengine.KindInternal, Message: "internal error"}
			}
			failure = describe(je)
			return nil, errors.New(failure)
		}
		if r.Validate != nil {
			if failure = r.Validate(out); failure != "" {
				return nil, errors.New(failure)
			}
		}
		res = out
		return nil, nil
	})
	if err != nil && failure == "" {
		s.log.Warn("dbmcp: script op failed", "scope", "dbmcp", "stage", string(r.Stage), "err", err)
		failure = describe(&jsengine.Error{Kind: jsengine.KindInternal, Message: "internal error"})
	}
	return res, failure
}

// visibleScript makes a source that failed Check safe to show: invalid UTF-8 becomes U+FFFD, NUL and
// bidirectional controls become visible \uXXXX escapes, so the op log never renders Trojan Source text.
func visibleScript(src string) string {
	var b strings.Builder
	for _, r := range strings.ToValidUTF8(src, "�") {
		if r == 0 || r >= 0x202A && r <= 0x202E || r >= 0x2066 && r <= 0x2069 {
			fmt.Fprintf(&b, `\u%04x`, r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
