package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/memory"
	"github.com/kirathecat/kira-studio/internal/memory/importer"
	"github.com/kirathecat/kira-studio/internal/memory/memfmt"
)

// TestMemoryIngestEval measures the memory ingest pipeline on a pinned corpus of repo text (P285).
// Opt-in twice: it lives in this nested module and needs KIRA_MEMORY_EVAL=1, because a run costs
// dollars. Run with `bun run eval:memory`. Env:
//
//	KIRA_EVAL_MODE     prepare | golden | run | footprint (free: re-renders finished rounds, no claude)
//	                   P291: questions | keys | telegraph | score (paid); retrieval | compare (free)
//	KIRA_EVAL_ROUNDS   footprint, keys, telegraph, score, retrieval, compare: comma-separated round names
//	                   (a bare name lives in the runs dir; "<dir>/<round>" under the eval data dir)
//	KIRA_EVAL_RUNS_DIR runs dir name under the eval data dir (default P285-runs; P291 uses P291-runs)
//	KIRA_EVAL_MODEL_HOME  home of the embedding model files (retrieval; default <user cache>/kira-eval)
//	KIRA_EVAL_RECIPES  retrieval: embedding recipes (fact, fact-kw, sec-fact, sec-fact-kw, sa-fact, sa-fact-kw)
//	KIRA_EVAL_STYLES   retrieval: full, tele, tele-guard, mixed, mixed-swap (default full)
//	KIRA_EVAL_STYLE    footprint, score: tele judges or sizes a round's telegraph facts
//	KIRA_EVAL_VARIANTS compare: two variant keys, one per side
//	KIRA_EVAL_EMBED_WORKER  internal: the test binary re-executed as the embedding worker
//	KIRA_EVAL_SET      tuning | holdout | all (run default tuning; prepare and golden default all)
//	KIRA_EVAL_ROUND    round name, the output dir under the eval data dir P285-runs
//	KIRA_EVAL_PROMPTS  JSON map {prompt id: text} overriding registry prompts for the run
//	KIRA_EVAL_BUDGET   chunk budget "Target,Max,MinFill,MinTail,Context"
//	KIRA_EVAL_ONLY     comma-separated source ids to run (smoke test); default all of the set
//	KIRA_EVAL_PARALLEL sources at once (default 2)
//	KIRA_EVAL_CAP_USD  run cap (default 8); KIRA_EVAL_PHASE_CAP_USD phase cap (default 60)
//	KIRA_EVAL_LEDGER   spend ledger file name under the runs dir (default spend.jsonl; one per part)
func TestMemoryIngestEval(t *testing.T) {
	if os.Getenv("KIRA_MEMORY_EVAL") != "1" {
		t.Skip("memory ingest eval: set KIRA_MEMORY_EVAL=1 (costs real money, see bun run eval:memory)")
	}
	man, err := loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	mode := os.Getenv("KIRA_EVAL_MODE")
	switch mode {
	case "footprint":
		evalFootprint(t, man)
		return
	case "retrieval":
		evalRetrieval(t, man)
		return
	case "compare":
		evalCompare(t)
		return
	}
	real, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("memory ingest eval: no claude on PATH")
	}
	switch mode {
	case "prepare":
		evalPrepare(t, man)
	case "golden":
		evalGolden(t, man, envOr("KIRA_EVAL_SET", "all"))
	case "run":
		evalRun(t, man, real)
	case "questions":
		evalQuestions(t, man)
	case "keys":
		evalKeys(t, man)
	case "telegraph":
		evalTelegraph(t, man)
	case "score":
		evalScoreTele(t, man)
	default:
		t.Fatalf("KIRA_EVAL_MODE = %q, want prepare, golden, run, footprint, questions, keys, telegraph, score, retrieval or compare", mode)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(k), 64); err == nil {
		return v
	}
	return def
}

func evalPrepare(t *testing.T, man evalManifest) {
	for i, s := range man.Sources {
		text, err := materialise(man.Commit, s)
		if err != nil {
			t.Fatal(err)
		}
		man.Sources[i].Bytes = len(text)
		t.Logf("%s %s %d bytes", s.Set, s.ID, len(text))
	}
	raw, _ := json.MarshalIndent(man, "", "  ")
	if err := os.WriteFile(filepath.Join(evalCorpusDir(), "manifest.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func evalGolden(t *testing.T, man evalManifest, set string) {
	curRound = "golden"
	have, _ := loadGolden()
	path := filepath.Join(evalCorpusDir(), "golden.jsonl")
	for _, s := range man.pick(set) {
		if _, ok := have[s.ID]; ok {
			continue
		}
		if runSpent() >= 8 {
			t.Fatalf("golden run cap reached at %.2f USD", runSpent())
		}
		text, err := materialise(man.Commit, s)
		if err != nil {
			t.Fatal(err)
		}
		g, err := authorGolden(context.Background(), s, text)
		if err != nil {
			t.Fatalf("%s: %v", s.ID, err)
		}
		line, _ := json.Marshal(g)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write(append(line, '\n'))
		_ = f.Close()
		t.Logf("golden %s: %d facts, %d dropped by the quote check, model %s, %.3f USD", s.ID, len(g.Facts), len(g.Dropped), g.Model, g.CostUSD)
	}
}

// Pipeline records.

type storedFact struct {
	ID       string   `json:"id"`
	Fact     string   `json:"fact"`
	Reason   string   `json:"reason"`
	Keywords []string `json:"keywords"`
}

type extractedFact struct {
	ID         string `json:"id"`
	Chunk      int    `json:"chunk"`
	Fact       string `json:"fact"`
	Evidence   string `json:"evidence"`
	Section    string `json:"section"`
	Unresolved string `json:"unresolved"`
}

type stageInfo struct {
	Chunks    int `json:"chunks"`
	Extracted int `json:"extracted"`
	Flagged   int `json:"flaggedUnresolved"`
	// Questionable counts facts sent to the cross-check; AfterCrossCheck the facts finalize received.
	Questionable    int                       `json:"questionable"`
	AfterCrossCheck int                       `json:"afterCrossCheck"`
	Verdicts        map[string]int            `json:"verdicts"`
	Dropped         []importer.DroppedFact    `json:"dropped"`
	Unresolved      []importer.UnresolvedFact `json:"unresolved"`
	Added           int                       `json:"added"`
	Updated         int                       `json:"updated"`
	Noop            int                       `json:"noop"`
	// Locator counts stored reasons that equal this file's source locator (Part 3 stores nothing else
	// from the source in a reason).
	Locator int `json:"locator"`
}

type runRecord struct {
	ID        string          `json:"id"`
	Stored    []storedFact    `json:"stored"`
	Extracted []extractedFact `json:"extracted"`
	Stages    stageInfo       `json:"stages"`
	MemoryDoc string          `json:"memoryDoc"`
	DocError  string          `json:"docError,omitempty"`
	// RenderedDoc is what importer.RenderDoc renders from the stored facts (C_doc reads it); MemoryDoc is
	// the file the write-back wrote, empty when the write failed. DocMismatch: both exist and differ.
	RenderedDoc    string `json:"renderedDoc,omitempty"`
	RenderProblems int    `json:"renderProblems,omitempty"`
	DocMismatch    bool   `json:"docMismatch,omitempty"`
	// CostUSD is the job (extract, cross-check, finalize); GateUSD the store_memory gate and reconcile calls.
	CostUSD    float64 `json:"costUsd"`
	ExtractUSD float64 `json:"extractUsd"`
	GateUSD    float64 `json:"gateUsd"`
	WallSec    int     `json:"wallSec"`
}

type evalCfg struct {
	round, scratch, exe, promptsFile string
	budget                           importer.Budget
	parallel                         int
	runCap, phaseCap                 float64
}

func parseBudget(v string) (importer.Budget, error) {
	if v == "" {
		return importer.DefaultBudget, nil
	}
	parts := strings.Split(v, ",")
	if len(parts) != 5 {
		return importer.Budget{}, fmt.Errorf("KIRA_EVAL_BUDGET wants 5 numbers, got %q", v)
	}
	var n [5]int
	for i, p := range parts {
		x, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return importer.Budget{}, err
		}
		n[i] = x
	}
	return importer.Budget{Target: n[0], Max: n[1], MinFill: n[2], MinTail: n[3], Context: n[4]}, nil
}

// buildPromptsFile turns the simple override map into an llm-prompts.json the stores read.
func buildPromptsFile(mapPath, out string) error {
	raw, err := os.ReadFile(mapPath)
	if err != nil {
		return err
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	st := llmprompts.NewStore(out)
	var changes []llmprompts.Change
	for id, text := range m {
		def := llmprompts.Defaults().Resolve(llmprompts.ID(id)).Text
		changes = append(changes, llmprompts.Change{ID: llmprompts.ID(id), Text: text, BaseHash: llmprompts.Hash(def)})
	}
	_, err = st.Save(changes)
	return err
}

// installCostWrapper puts a `claude` first on PATH that copies stdout of runs whose env names
// KIRA_EVAL_COSTLOG into that dir. Only the memory-mcp child (the store_memory gate) carries the
// variable: the job's own cost is already reported by the importer.
func installCostWrapper(t *testing.T, real string) {
	dir := t.TempDir()
	script := fmt.Sprintf(`#!/bin/bash
if [ -n "$KIRA_EVAL_COSTLOG" ]; then
  f=$(mktemp "$KIRA_EVAL_COSTLOG/run.XXXXXX")
  %q "$@" | tee "$f"
  exit "${PIPESTATUS[0]}"
fi
exec %q "$@"
`, real, real)
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func costLogTotal(dir string) float64 {
	entries, _ := os.ReadDir(dir)
	var total float64
	for _, e := range entries {
		raw, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		var r struct {
			Cost float64 `json:"total_cost_usd"`
		}
		if json.Unmarshal(raw, &r) == nil {
			total += r.Cost
		}
	}
	return total
}

func evalRun(t *testing.T, man evalManifest, realClaude string) {
	round := os.Getenv("KIRA_EVAL_ROUND")
	if round == "" {
		t.Fatal("KIRA_EVAL_ROUND is required")
	}
	curRound = round
	cfg := evalCfg{round: round, parallel: int(envFloat("KIRA_EVAL_PARALLEL", 2)),
		runCap: envFloat("KIRA_EVAL_CAP_USD", 8), phaseCap: envFloat("KIRA_EVAL_PHASE_CAP_USD", 60), scratch: t.TempDir()}
	var err error
	if cfg.budget, err = parseBudget(os.Getenv("KIRA_EVAL_BUDGET")); err != nil {
		t.Fatal(err)
	}
	if cfg.exe, err = os.Executable(); err != nil {
		t.Fatal(err)
	}
	if p := os.Getenv("KIRA_EVAL_PROMPTS"); p != "" {
		cfg.promptsFile = filepath.Join(cfg.scratch, "llm-prompts.json")
		if err := buildPromptsFile(p, cfg.promptsFile); err != nil {
			t.Fatalf("prompts override: %v", err)
		}
	}
	installCostWrapper(t, realClaude)
	golden, err := loadGolden()
	if err != nil {
		t.Fatalf("golden.jsonl: %v", err)
	}
	set := envOr("KIRA_EVAL_SET", "tuning")
	srcs := man.pick(set)
	if only := os.Getenv("KIRA_EVAL_ONLY"); only != "" {
		var keep []evalSource
		for _, s := range srcs {
			if slices.Contains(strings.Split(only, ","), s.ID) {
				keep = append(keep, s)
			}
		}
		srcs = keep
	}
	for _, s := range srcs {
		if _, ok := golden[s.ID]; !ok {
			t.Fatalf("no golden facts for %s", s.ID)
		}
	}
	roundDir := filepath.Join(evalRunsDir(), round)
	if err := os.MkdirAll(filepath.Join(roundDir, "judge"), 0o755); err != nil {
		t.Fatal(err)
	}
	recs := loadRecords(filepath.Join(roundDir, "facts.jsonl"))
	var mu sync.Mutex
	capped := false
	over := func() bool {
		mu.Lock()
		defer mu.Unlock()
		if runSpent() >= cfg.runCap || phaseSpent() >= cfg.phaseCap-2 {
			capped = true
		}
		return capped
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, cfg.parallel)
	grades := map[string]scoreGrades{}
	texts := map[string]string{}
	judgeModel := ""
	failed := map[string]string{}
	for _, s := range srcs {
		text, err := materialise(man.Commit, s)
		if err != nil {
			t.Fatal(err)
		}
		texts[s.ID] = text
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			fail := func(msg string) {
				mu.Lock()
				failed[s.ID] = msg
				mu.Unlock()
				t.Logf("%s: %s", s.ID, msg)
			}
			mu.Lock()
			rec, have := recs[s.ID]
			mu.Unlock()
			if !have {
				if over() {
					fail("skipped: spend cap")
					return
				}
				begin := time.Now()
				// A transient CLI failure fails the file; rerun the whole source once on a fresh store.
				r, err := runPipeline(t, cfg, s, text)
				if err != nil && !over() {
					t.Logf("%s: retrying after: %v", s.ID, err)
					r, err = runPipeline(t, cfg, s, text)
				}
				if err != nil {
					fail("pipeline: " + err.Error())
					return
				}
				r.WallSec = int(time.Since(begin).Seconds())
				rec = r
				mu.Lock()
				recs[s.ID] = r
				appendRecord(filepath.Join(roundDir, "facts.jsonl"), r)
				mu.Unlock()
				t.Logf("%s: stored %d, extracted %d, %.3f USD job + %.3f gate, %ds", s.ID, len(r.Stored), len(r.Extracted), r.CostUSD, r.GateUSD, r.WallSec)
			}
			jp := filepath.Join(roundDir, "judge", s.ID+".json")
			var cached struct {
				Model  string      `json:"model"`
				Grades scoreGrades `json:"grades"`
			}
			if raw, err := os.ReadFile(jp); err == nil && json.Unmarshal(raw, &cached) == nil && validateGrades(cached.Grades, golden[s.ID].Facts, rec.Stored) == nil {
				mu.Lock()
				grades[s.ID], judgeModel = cached.Grades, cached.Model
				mu.Unlock()
				return
			}
			if over() {
				fail("skipped: spend cap before scoring")
				return
			}
			g, model, err := scoreSource(context.Background(), s, text, golden[s.ID], rec)
			if err != nil {
				fail("score: " + err.Error())
				return
			}
			raw, _ := json.MarshalIndent(map[string]any{"model": model, "grades": g}, "", "  ")
			_ = os.WriteFile(jp, append(raw, '\n'), 0o644)
			mu.Lock()
			grades[s.ID], judgeModel = g, model
			mu.Unlock()
		}()
	}
	wg.Wait()
	writeScores(t, roundDir, "scores.json", cfg, set, srcs, texts, golden, recs, grades, judgeModel, capped, failed)
	t.Logf("run spend %.3f USD, phase total %.3f USD", runSpent(), phaseSpent())
	if len(failed) > 0 {
		t.Fatalf("incomplete round: %v", failed)
	}
}

func loadRecords(path string) map[string]runRecord {
	out := map[string]runRecord{}
	raw, _ := os.ReadFile(path)
	for _, l := range strings.Split(string(raw), "\n") {
		var r runRecord
		if strings.TrimSpace(l) != "" && json.Unmarshal([]byte(l), &r) == nil {
			out[r.ID] = r
		}
	}
	return out
}

func appendRecord(path string, r runRecord) {
	line, _ := json.Marshal(r)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// runPipeline ingests one source into a fresh store and collects what each stage produced.
func runPipeline(t *testing.T, cfg evalCfg, s evalSource, text string) (runRecord, error) {
	dir, _ := os.MkdirTemp(cfg.scratch, s.ID+"-")
	srcDir, home, costDir := filepath.Join(dir, "src"), filepath.Join(dir, "memory"), filepath.Join(dir, "costlog")
	for _, d := range []string{srcDir, home, costDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return runRecord{}, err
		}
	}
	if err := os.WriteFile(filepath.Join(srcDir, s.fileName()), []byte(text), 0o644); err != nil {
		return runRecord{}, err
	}
	ms := memory.NewStore(filepath.Join(home, "memory.db"))
	defer ms.Close()
	var prompts llmprompts.Source
	if cfg.promptsFile != "" {
		prompts = llmprompts.NewStore(cfg.promptsFile)
	}
	svc := memory.NewService(ms, newAgentRunner(), memory.ServiceOptions{Prompts: prompts})
	defer svc.Close()
	inner := importer.ModelAgent{Runner: newAgentRunner(), Executable: cfg.exe, SpaceHome: dir, Prompts: prompts,
		Env: map[string]string{"KIRA_TEST_MEMORY_MCP": "1", "KIRA_TEST_PROMPTS": cfg.promptsFile, "KIRA_EVAL_COSTLOG": costDir}}
	ra := &recAgent{inner: inner, raw: map[int][]importer.Fact{}, verdicts: map[string]int{}}
	runner, err := importer.NewRunner(ms, importer.RunnerOptions{Agent: ra, Docs: svc, Budget: cfg.budget})
	if err != nil {
		return runRecord{}, err
	}
	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); runRunner(runCtx, runner) }()
	defer func() { stop(); <-done }()
	e, err := importer.Open(ms, importer.Options{Launcher: noLauncher{}})
	if err != nil {
		return runRecord{}, err
	}
	defer e.Close()
	job, err := e.Create(importer.CreateArgs{Paths: []string{srcDir}, WriteDoc: true,
		Provenance: importer.Provenance{Origin: memory.OriginAgent, Scope: memory.ScopeProject, Project: "github.com/kirathecat/kira-studio"}})
	if err != nil {
		return runRecord{}, err
	}
	wait := func(want string, limit time.Duration) (importer.JobDetail, error) {
		deadline := time.Now().Add(limit)
		for time.Now().Before(deadline) {
			d, err := e.Job(job.ID)
			if err != nil {
				return d, err
			}
			if d.Job.State == want {
				return d, nil
			}
			if d.Job.State == importer.JobFailed || d.Job.State == importer.JobPaused || d.Job.State == importer.JobCancelled {
				return d, fmt.Errorf("job %s: %s", d.Job.State, d.Job.Reason)
			}
			time.Sleep(500 * time.Millisecond)
		}
		return importer.JobDetail{}, fmt.Errorf("job never reached %s", want)
	}
	if _, err := wait(importer.JobAwaiting, 3*time.Minute); err != nil {
		return runRecord{}, err
	}
	if err := e.Start(job.ID, importer.StartArgs{}); err != nil {
		return runRecord{}, err
	}
	d, err := wait(importer.JobDone, 12*time.Minute)
	if err != nil {
		evalSpend("pipeline", s.ID, d.Job.CostUSD)
		evalSpend("gate", s.ID, costLogTotal(costDir))
		return runRecord{}, err
	}
	rec := runRecord{ID: s.ID, CostUSD: d.Job.CostUSD, GateUSD: costLogTotal(costDir)}
	evalSpend("pipeline", s.ID, rec.CostUSD)
	evalSpend("gate", s.ID, rec.GateUSD)
	if len(d.Files) != 1 || d.Files[0].State != importer.FileDone {
		return runRecord{}, fmt.Errorf("file not done: %+v", d.Files)
	}
	// The write-back runs just after the file is done; wait for its path (or error).
	for range 60 {
		if d.Files[0].DocPath != "" || d.Files[0].DocError != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
		if d, err = e.Job(job.ID); err != nil {
			return runRecord{}, err
		}
	}
	f := d.Files[0]
	rec0DocError := f.DocError
	ctx := context.Background()
	mems, err := ms.DocumentFacts(ctx, f.ID)
	if err != nil {
		return runRecord{}, err
	}
	for i, m := range mems {
		rec.Stored = append(rec.Stored, storedFact{ID: "s" + strconv.Itoa(i+1), Fact: m.Fact, Reason: m.Reason, Keywords: m.Keywords})
	}
	rec.ExtractUSD = f.CostUSD
	ra.fill(&rec)
	rec.DocError = rec0DocError
	rec.Stages.Chunks = f.ChunkCount
	rec.Stages.Dropped, rec.Stages.Unresolved = f.Dropped, f.Unresolved
	rec.Stages.Added, rec.Stages.Updated, rec.Stages.Noop = f.Added, f.Updated, f.Noop
	updated := evalDocDate
	if f.DocPath != "" {
		if raw, err := os.ReadFile(f.DocPath); err == nil {
			rec.MemoryDoc = string(raw)
			if doc, ok, _ := memfmt.ParseDoc(raw); ok && doc.Updated != "" {
				updated = doc.Updated
			}
		}
	}
	for _, m := range mems {
		if _, ok := memfmt.ParseLocator(m.Reason, f.RelPath); ok {
			rec.Stages.Locator++
		}
	}
	rec.RenderedDoc, rec.RenderProblems = renderChecked(importer.DocInput{GroupID: f.GroupID, Title: f.Title, FileName: s.fileName(), Rel: f.RelPath,
		Prov: evalProvenance(), Updated: updated}, mems)
	rec.DocMismatch = rec.MemoryDoc != "" && rec.MemoryDoc != rec.RenderedDoc
	return rec, nil
}

func writeScores(t *testing.T, roundDir, file string, cfg evalCfg, set string, srcs []evalSource, texts map[string]string,
	golden map[string]goldenLine, recs map[string]runRecord, grades map[string]scoreGrades, judgeModel string, capped bool, failed map[string]string) {
	type missed struct {
		ID          string `json:"id"`
		Fact        string `json:"fact"`
		Covered     string `json:"covered"`
		InExtracted bool   `json:"inExtracted"`
	}
	type srcOut struct {
		Category string `json:"category"`
		Tally    tally  `json:"tally"`
		Ratios   ratios `json:"ratios"`
		// MissedEssential lists essential golden facts with no or partial coverage.
		MissedEssential []missed `json:"missedEssential"`
		// Notes are the judge's notes on failed checks of stored facts.
		Notes []string `json:"notes"`
		// DocMismatch: the written .memory.md differs from the render; RenderProblems: ParseDoc problems
		// of the render; DocError: why the write-back failed, if it did.
		DocMismatch    bool    `json:"docMismatch"`
		LocatorShare   float64 `json:"locatorShare"`
		RenderProblems int     `json:"renderProblems"`
		DocError       string  `json:"docError,omitempty"`
	}
	out := map[string]any{"round": cfg.round, "set": set, "judgeModel": judgeModel, "capped": capped, "incomplete": failed,
		"budget": cfg.budget, "promptsOverride": cfg.promptsFile != ""}
	per := map[string]srcOut{}
	var agg tally
	var ids []string
	for _, s := range srcs {
		g, ok := grades[s.ID]
		if !ok {
			continue
		}
		rec := recs[s.ID]
		tl := tallyOf(texts[s.ID], golden[s.ID], rec, g)
		so := srcOut{Category: s.Category, Tally: tl, Ratios: tl.ratios(), DocMismatch: rec.DocMismatch,
			RenderProblems: rec.RenderProblems, DocError: rec.DocError,
			LocatorShare: div(float64(rec.Stages.Locator), float64(len(rec.Stored)))}
		weight := map[string]string{}
		fact := map[string]string{}
		for _, f := range golden[s.ID].Facts {
			weight[f.ID], fact[f.ID] = f.Weight, f.Fact
		}
		for _, x := range g.Golden {
			if weight[x.ID] == "essential" && x.Covered != "full" {
				so.MissedEssential = append(so.MissedEssential, missed{x.ID, fact[x.ID], x.Covered, x.InExtracted})
			}
		}
		for _, x := range g.Stored {
			if x.Note != "" {
				so.Notes = append(so.Notes, x.ID+": "+x.Note)
			}
		}
		per[s.ID] = so
		agg.add(tl)
		ids = append(ids, s.ID)
	}
	slices.Sort(ids)
	out["sources"] = per
	var loc, stored int
	for _, id := range ids {
		loc += recs[id].Stages.Locator
		stored += len(recs[id].Stored)
	}
	out["aggregate"] = map[string]any{"tally": agg, "ratios": agg.ratios(), "locatorShare": div(float64(loc), float64(stored))}
	var pipe, gate, extract float64
	for _, r := range recs {
		pipe += r.CostUSD
		gate += r.GateUSD
		extract += r.ExtractUSD
	}
	out["costUsd"] = map[string]float64{"job": pipe, "gate": gate, "extractShareOfJob": extract}
	out["thisProcessSpendUsd"] = runSpent()
	raw, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(filepath.Join(roundDir, file), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	r := agg.ratios()
	t.Logf("%s: R_ess %.3f R_all %.3f P %.3f unsupported %.3f dup %.3f C_fact %.3f (stored %d)", cfg.round, r.REss, r.RAll, r.P, r.Unsupported, r.Duplicate, r.CFact, agg.Stored)
}

// recAgent records what each stage hands the next: the chunk's facts are deleted from memory.db when
// the file finishes, so the harness keeps them here. Extracted in the record is what finalize received
// (extracted and cross-checked, not yet merged).
type recAgent struct {
	inner importer.ModelAgent
	mu    sync.Mutex
	raw   map[int][]importer.Fact
	// questionable counts facts sent to the cross-check; verdicts counts them by kind.
	questionable int
	verdicts     map[string]int
	final        []importer.FinalFact
}

func (r *recAgent) Available() error { return r.inner.Available() }

func (r *recAgent) Extract(ctx context.Context, in importer.ExtractInput) (importer.ExtractOutput, error) {
	out, err := r.inner.Extract(ctx, in)
	if err == nil {
		r.mu.Lock()
		r.raw[in.Index] = out.Facts
		r.mu.Unlock()
	}
	return out, err
}

func (r *recAgent) Verify(ctx context.Context, in importer.VerifyInput) (importer.VerifyOutput, error) {
	out, err := r.inner.Verify(ctx, in)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err == nil {
		r.questionable += len(in.Facts)
		for _, v := range out.Verdicts {
			r.verdicts[v.Verdict]++
		}
	}
	return out, err
}

func (r *recAgent) Finalize(ctx context.Context, in importer.FinalizeInput) (importer.FinalizeOutput, error) {
	r.mu.Lock()
	r.final = in.Facts
	r.mu.Unlock()
	return r.inner.Finalize(ctx, in)
}

// Merge passes through: the runner never calls it; the recorder satisfies importer.Agent.
func (r *recAgent) Merge(ctx context.Context, in importer.MergeInput) (importer.MergeOutput, error) {
	return r.inner.Merge(ctx, in)
}

func (r *recAgent) fill(rec *runRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, f := range r.final {
		rec.Extracted = append(rec.Extracted, extractedFact{ID: "e" + f.ID, Chunk: f.Chunk, Fact: f.Fact.Fact,
			Evidence: f.Evidence, Section: f.Section, Unresolved: f.Unresolved})
	}
	for _, facts := range r.raw {
		rec.Stages.Extracted += len(facts)
		for _, f := range facts {
			if f.Unresolved != "" {
				rec.Stages.Flagged++
			}
		}
	}
	rec.Stages.Questionable = r.questionable
	rec.Stages.Verdicts = r.verdicts
	rec.Stages.AfterCrossCheck = len(r.final)
}
