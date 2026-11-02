package ade

import (
	"reflect"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func view(def stepDef, targets []string, runs ...model.AdeRun) stepView {
	v := stepView{Def: def, Targets: targets, Runs: map[string]model.AdeRun{}}
	for _, r := range runs {
		v.Runs[r.BranchID] = r
	}
	return v
}

func run(branch, state string, attempt int) model.AdeRun {
	return model.AdeRun{ID: branch + state, BranchID: branch, State: state, Attempt: attempt}
}

func TestStepState(t *testing.T) {
	targets := []string{"a", "b"}
	for name, tc := range map[string]struct {
		runs []model.AdeRun
		want string
	}{
		"no runs":       {nil, model.AdeRunPending},
		"all done":      {[]model.AdeRun{run("a", "done", 1), run("b", "done", 1)}, model.AdeRunDone},
		"done+pending":  {[]model.AdeRun{run("a", "done", 1), run("b", "pending", 1)}, model.AdeRunPending},
		"back is run":   {[]model.AdeRun{run("a", "done", 1), run("b", "back", 1)}, model.AdeRunRunning},
		"failed>run":    {[]model.AdeRun{run("a", "running", 1), run("b", "failed", 1)}, model.AdeRunFailed},
		"stuck>failed":  {[]model.AdeRun{run("a", "stuck", 1), run("b", "failed", 1)}, model.AdeRunStuck},
		"one of two":    {[]model.AdeRun{run("a", "done", 1)}, model.AdeRunPending},
		"running+done":  {[]model.AdeRun{run("a", "done", 1), run("b", "running", 1)}, model.AdeRunRunning},
		"stuck+running": {[]model.AdeRun{run("a", "running", 1), run("b", "stuck", 1)}, model.AdeRunStuck},
	} {
		if got := view(stepDef{}, targets, tc.runs...).state(); got != tc.want {
			t.Errorf("%s: state = %s, want %s", name, got, tc.want)
		}
	}
}

func TestNextAction(t *testing.T) {
	one := []string{"a"}
	auto := stepDef{ID: "s", Before: "auto"}
	gated := stepDef{ID: "g", Before: "approval"}
	retry2 := stepDef{ID: "r", Before: "auto"}
	failing := stepDef{ID: "b", Before: "auto"}
	manual := stepDef{ID: "m", Kind: "manual"}
	done := view(auto, one, run("a", "done", 1))

	for name, tc := range map[string]struct {
		steps []stepView
		want  action
	}{
		"first step never auto-starts":      {[]stepView{view(gated, one)}, action{Kind: actIdle, Step: 0}},
		"next step starts":                  {[]stepView{done, view(auto, one)}, action{Kind: actStart, Step: 1}},
		"gate waits for approval":           {[]stepView{done, view(gated, one)}, action{Kind: actAwaitApproval, Step: 1}},
		"pending with runs waits on setup":  {[]stepView{view(auto, one, run("a", "pending", 1))}, action{Kind: actIdle, Step: 0}},
		"running waits":                     {[]stepView{view(auto, one, run("a", "running", 1)), view(auto, one)}, action{Kind: actIdle, Step: 0}},
		"stuck waits for a person":          {[]stepView{view(retry2, one, run("a", "stuck", 1))}, action{Kind: actIdle, Step: 0}},
		"stop leaves failed":                {[]stepView{view(auto, one, run("a", "failed", 1))}, action{Kind: actIdle, Step: 0}},
		"a failed step idles":               {[]stepView{done, view(failing, one, run("a", "failed", 1))}, action{Kind: actIdle, Step: 1}},
		"all done completes the workflow":   {[]stepView{done, view(auto, one, run("a", "done", 2))}, action{Kind: actComplete, Step: 1}},
		"a manual step waits for the user":  {[]stepView{done, view(manual, nil)}, action{Kind: actWaitUser, Step: 1}},
		"no steps completes":                {nil, action{Kind: actComplete}},
		"done step before a stuck one idle": {[]stepView{done, view(auto, one, run("a", "stuck", 1)), view(auto, one)}, action{Kind: actIdle, Step: 1}},
	} {
		if got := nextAction(tc.steps, 0); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: action = %+v, want %+v", name, got, tc.want)
		}
	}
}

func TestStepTargets(t *testing.T) {
	mine := []mineBranch{{"b1", "api"}, {"b2", "web"}, {"b3", "api"}}
	for name, tc := range map[string]struct {
		runsOn string
		want   []string
		fails  bool
	}{
		"once is the first":  {"once", []string{"b1"}, false},
		"each repo is all":   {"each repo", []string{"b1", "b2", "b3"}, false},
		"only by nickname":   {"only api", []string{"b1", "b3"}, false},
		"only unresolved":    {"only db", nil, true},
		"empty runsOn = all": {"", []string{"b1", "b2", "b3"}, false},
	} {
		got, err := stepTargets(tc.runsOn, false, mine)
		if (err != nil) != tc.fails || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, %v", name, got, err)
		}
	}
	for name, tc := range map[string]struct {
		runsOn string
		review bool
		want   []string
		fails  bool
	}{
		"once":        {"once", false, []string{taskTarget}, false},
		"each repo":   {"", false, []string{taskTarget}, false},
		"only":        {"only api", false, nil, true},
		"review step": {"", true, nil, true},
	} {
		got, err := stepTargets(tc.runsOn, tc.review, nil)
		if (err != nil) != tc.fails || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("no branch, %s: got %v, %v", name, got, err)
		}
	}
}

func loopRun(branch, state string, attempt, loops int) model.AdeRun {
	r := run(branch, state, attempt)
	r.Loops = loops
	return r
}

func TestChainRerun(t *testing.T) {
	one := []string{"a"}
	impl := stepDef{ID: "impl", Before: "auto"}
	tests := stepDef{ID: "tests", Before: "auto"}
	ci := stepDef{ID: "ci", Before: "auto"}
	gated := stepDef{ID: "gate", Before: "approval"}

	for name, tc := range map[string]struct {
		steps []stepView
		want  []rerun
	}{
		"before any send-back nothing reruns": {
			[]stepView{view(impl, one, loopRun("a", "done", 1, 0)), view(tests, one, loopRun("a", "failed", 1, 0))}, nil},
		"fix still running waits": {
			[]stepView{view(impl, one, loopRun("a", "running", 2, 1)), view(tests, one, loopRun("a", "back", 1, 0))}, nil},
		"back step reruns after the fix": {
			[]stepView{view(impl, one, loopRun("a", "done", 2, 1)), view(tests, one, loopRun("a", "back", 1, 0))},
			[]rerun{{Step: 1, Branch: "a", Attempt: 2, Loops: 1}}},
		"steps between target and failer rerun first": {
			[]stepView{view(impl, one, loopRun("a", "done", 2, 1)), view(tests, one, loopRun("a", "done", 1, 0)), view(ci, one, loopRun("a", "back", 1, 0))},
			[]rerun{{Step: 1, Branch: "a", Attempt: 2, Loops: 1}}},
		"a rerun in flight ends the walk": {
			[]stepView{view(impl, one, loopRun("a", "done", 2, 1)), view(tests, one, loopRun("a", "running", 2, 1)), view(ci, one, loopRun("a", "back", 1, 0))}, nil},
		"current rounds are not stale": {
			[]stepView{view(impl, one, loopRun("a", "done", 2, 1)), view(tests, one, loopRun("a", "done", 2, 1)), view(ci, one, loopRun("a", "back", 1, 0))},
			[]rerun{{Step: 2, Branch: "a", Attempt: 2, Loops: 1}}},
		"second round uses the higher loops": {
			[]stepView{view(impl, one, loopRun("a", "done", 3, 2)), view(tests, one, loopRun("a", "back", 2, 1))},
			[]rerun{{Step: 1, Branch: "a", Attempt: 3, Loops: 2}}},
		"a missing run stops the walk": {
			[]stepView{view(impl, one, loopRun("a", "done", 2, 1)), view(gated, one), view(tests, one, loopRun("a", "back", 1, 0))}, nil},
		"approval is not consulted for a rerun": {
			[]stepView{view(impl, one, loopRun("a", "done", 2, 1)), view(gated, one, loopRun("a", "done", 1, 0))},
			[]rerun{{Step: 1, Branch: "a", Attempt: 2, Loops: 1}}},
		"a failed run is left to nextAction": {
			[]stepView{view(impl, one, loopRun("a", "done", 2, 1)), view(tests, one, loopRun("a", "failed", 1, 0))}, nil},
		"each branch walks on its own": {
			[]stepView{view(impl, []string{"a", "b"}, loopRun("a", "done", 2, 1), loopRun("b", "done", 1, 0)),
				view(tests, []string{"a", "b"}, loopRun("a", "back", 1, 0), loopRun("b", "done", 1, 0))},
			[]rerun{{Step: 1, Branch: "a", Attempt: 2, Loops: 1}}},
	} {
		if got := chainRerun(tc.steps, 0); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: reruns = %+v, want %+v", name, got, tc.want)
		}
	}
}

func routed(branch string, attempt int, route string) model.AdeRun {
	r := run(branch, "done", attempt)
	r.Outcome = &model.AdeRunOutcome{Route: route}
	return r
}

func TestWalkPath(t *testing.T) {
	one := []string{"a", "b"}
	def := func(id string) stepDef { return stepDef{ID: id, Before: "auto"} }
	for name, tc := range map[string]struct {
		steps   []stepView
		path    []int
		skipped []bool
	}{
		"legacy runs go to the next step": {
			[]stepView{view(def("x"), one, routed("a", 1, ""), routed("b", 1, "")), view(def("y"), one)}, []int{0, 1}, []bool{false, false}},
		"a forward route skips steps": {
			[]stepView{view(def("x"), one, routed("a", 1, "z"), routed("b", 1, "z")), view(def("y"), one), view(def("z"), one)}, []int{0, 2}, []bool{false, true, false}},
		"end completes the workflow": {
			[]stepView{view(def("x"), one, routed("a", 1, "end"), routed("b", 1, "end")), view(def("y"), one)}, []int{0}, []bool{false, true}},
		"the earliest forward target across branches wins": {
			[]stepView{view(def("x"), one, routed("a", 1, "z"), routed("b", 1, "next")), view(def("y"), one), view(def("z"), one)}, []int{0, 1}, []bool{false, false, false}},
		"the path ends at the first step not done": {
			[]stepView{view(def("x"), one, routed("a", 1, "next"), routed("b", 1, "next")), view(def("y"), one, run("a", "running", 1)), view(def("z"), one)}, []int{0, 1}, []bool{false, false, false}},
	} {
		path, skipped := walkPath(tc.steps, 0)
		if !reflect.DeepEqual(path, tc.path) || !reflect.DeepEqual(skipped, tc.skipped) {
			t.Errorf("%s: path %v skipped %v, want %v %v", name, path, skipped, tc.path, tc.skipped)
		}
	}
}

func TestNextAction_route(t *testing.T) {
	one := []string{"a"}
	def := func(id string) stepDef { return stepDef{ID: id, Before: "auto"} }
	skipTo := []stepView{view(def("x"), one, routed("a", 1, "z")), view(def("y"), one), view(def("z"), one)}
	if got := nextAction(skipTo, 0); got != (action{Kind: actStart, Step: 2}) {
		t.Errorf("a skipped step started: %+v", got)
	}
	end := []stepView{view(def("x"), one, routed("a", 1, "end")), view(def("y"), one)}
	if got := nextAction(end, 0); got.Kind != actComplete {
		t.Errorf("end did not complete the workflow: %+v", got)
	}
}

func TestChainRerun_route(t *testing.T) {
	one := []string{"a"}
	def := func(id string) stepDef { return stepDef{ID: id, Before: "auto"} }
	// review loops back to impl; docs was skipped by impl's route and is never queued again.
	steps := []stepView{
		view(def("impl"), one, func() model.AdeRun { r := routed("a", 2, "review"); r.Loops = 1; return r }()),
		view(def("docs"), one),
		view(def("review"), one, loopRun("a", "back", 1, 0)),
	}
	want := []rerun{{Step: 2, Branch: "a", Attempt: 2, Loops: 1}}
	if got := chainRerun(steps, 0); !reflect.DeepEqual(got, want) {
		t.Errorf("reruns = %+v, want %+v", got, want)
	}
}

func TestWalkPath_entry(t *testing.T) {
	one := []string{"a"}
	def := func(id string) stepDef { return stepDef{ID: id, Before: "auto"} }
	steps := []stepView{view(def("x"), one, routed("a", 1, "")), view(def("y"), one), view(def("z"), one)}
	path, _ := walkPath(steps, 1)
	if !reflect.DeepEqual(path, []int{1}) {
		t.Errorf("path from entry 1 = %v, want [1]", path)
	}
	if got := currentStep(steps, 1); got != 1 {
		t.Errorf("currentStep = %d, want 1", got)
	}
}
