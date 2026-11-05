package ade

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/kirathecat/kira-studio/internal/scripts"
)

// scriptUse says who references a Run script: the kind it must be and the file it is named in.
type scriptUse struct{ kind, file string }

var (
	workflowUse = scriptUse{scripts.KindSmart, "workflow file"}
	repoFileUse = scriptUse{scripts.KindScript, "repo file"}
)

// resolvedScript is a referenced Run script with its params, env and folder resolved.
type resolvedScript struct {
	script   scripts.CustomScript
	env      []string          // KIRA_<builtin> then KIRA_PARAM_<name>
	rendered map[string]string // builtins and non-secret params, the Compose input
	secrets  []string          // values for the log mask
	dir      string
}

// refReason is why a reference cannot resolve; callers prefix it with their own context.
type refReason string

func (r refReason) Error() string { return string(r) }

func refFail(format string, a ...any) error { return refReason(fmt.Sprintf(format, a...)) }

// resolveScriptRef finds the one script of use.kind named name and resolves its params, env and
// folder as a workflow step does. adeDir is where a script that runs in the ADE worktree runs. A
// problem with the reference is a refReason; any other error is a failure to read the scripts.
func (b *TaskBoard) resolveScriptRef(all []scripts.CustomScript, use scriptUse, name string, given map[string][]string, vars map[string]string, adeDir string) (*resolvedScript, error) {
	if b.deps.CustomScripts == nil {
		return nil, refFail("scripts are not available")
	}
	var found []scripts.CustomScript
	otherKind := false
	for _, s := range all {
		if s.Name != name {
			continue
		}
		if s.Kind == use.kind && (use.kind != scripts.KindSmart || s.Smart != nil) {
			found = append(found, s)
		} else {
			otherKind = true
		}
	}
	switch len(found) {
	case 0:
		if otherKind && use.kind == scripts.KindScript {
			return nil, refFail("is a smart script: a %s runs a normal script", use.file)
		}
		return nil, refFail("not found")
	case 1:
	default:
		return nil, refFail("%d scripts have that name", len(found))
	}
	script := found[0]
	params, err := b.secretParams(script, given, use.file)
	if err != nil {
		var r refReason
		if errors.As(err, &r) {
			return nil, r
		}
		return nil, refFail("%s", err)
	}
	vals, missing, err := scripts.ParamValues(script.Params, params)
	if err != nil {
		return nil, refFail("%s", err)
	}
	if len(missing) > 0 {
		return nil, refFail("needs a value for %s", strings.Join(missing, ", "))
	}
	rendered := maps.Clone(vars)
	env := []string{}
	for _, v := range scripts.BuiltinVars {
		env = append(env, scripts.BuiltinEnv(v)+"="+rendered[v])
	}
	res := &resolvedScript{script: script, dir: adeDir}
	for _, v := range vals {
		env = append(env, scripts.EnvName(v.Name)+"="+v.Env)
		if v.Secret {
			res.secrets = append(res.secrets, v.Env)
		} else {
			rendered[v.Name] = v.Rendered
		}
	}
	res.env, res.rendered = env, rendered
	if !script.UseAdeDir {
		dir := scripts.ResolveDir(script, b.deps.ScriptsHome)
		if err := scripts.PrepareDir(dir); err != nil {
			return nil, refFail("%s", err)
		}
		res.dir = dir.Path
	}
	return res, nil
}

// listScripts reads the Run scripts, once per caller.
func (b *TaskBoard) listScripts() ([]scripts.CustomScript, error) {
	if b.deps.CustomScripts == nil {
		return nil, nil
	}
	return b.deps.CustomScripts.List()
}
