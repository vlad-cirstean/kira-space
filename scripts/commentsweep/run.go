package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/scripts/internal/runledger"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	reserveUSD  = 0.05
	callTimeout = 10 * time.Minute
	retryDelay  = 30 * time.Second
	progressN   = 50
)

type runOpts struct {
	maxUSD   float64
	jobs     int
	lang     string
	binary   string
	limit    int
	prefixes []string
}

// toolError is an error the tool itself reported; the MCP session stays usable after it.
type toolError struct{ msg string }

func (e toolError) Error() string { return e.msg }

type rewriteOut struct {
	CostUSD float64 `json:"costUsd"`
	Rewrite *struct {
		Text    string    `json:"text"`
		Changed bool      `json:"changed"`
		Gate    string    `json:"gate"`
		Reason  string    `json:"reason"`
		Dropped []Dropped `json:"dropped"`
	} `json:"rewrite"`
}

// worker owns one kira-space memory-mcp process and its MCP session, restarted when it dies.
type worker struct {
	bin, home string
	session   *mcp.ClientSession
}

func (w *worker) ensure(ctx context.Context) error {
	if w.session != nil {
		return nil
	}
	cmd := exec.Command(w.bin, "memory-mcp")
	cmd.Dir = w.home
	cmd.Env = append(os.Environ(), "KIRA_MEMORY_HOME="+filepath.Join(w.home, "memory"), "KIRA_SPACE_HOME="+filepath.Join(w.home, "space"))
	client := mcp.NewClient(&mcp.Implementation{Name: "commentsweep", Version: "1"}, nil)
	s, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	w.session = s
	return nil
}

func (w *worker) reset() {
	if w.session != nil {
		_ = w.session.Close()
		w.session = nil
	}
}

func (w *worker) call(ctx context.Context, text, codeCtx string) (rewriteOut, error) {
	if err := w.ensure(ctx); err != nil {
		return rewriteOut{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	res, err := w.session.CallTool(cctx, &mcp.CallToolParams{Name: "ingest_files",
		Arguments: map[string]any{"distill": true, "mode": "rewrite", "text": text, "context": codeCtx}})
	if err != nil {
		w.reset()
		return rewriteOut{}, err
	}
	if res.IsError {
		var msg []string
		for _, c := range res.Content {
			if t, ok := c.(*mcp.TextContent); ok {
				msg = append(msg, t.Text)
			}
		}
		return rewriteOut{}, toolError{strings.Join(msg, " ")}
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return rewriteOut{}, err
	}
	var out rewriteOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return rewriteOut{}, err
	}
	if out.Rewrite == nil {
		return out, toolError{"no rewrite in the tool output"}
	}
	return out, nil
}

func buildBinary(dir string) (string, error) {
	for _, d := range []string{"apps/kira-space/frontend/dist", "apps/kira-space/frontend/dist-mobile"} {
		if _, err := os.Stat(d); err != nil {
			fmt.Fprintln(os.Stderr, "building kira-space frontend")
			b := exec.Command("bun", "run", "build")
			b.Stdout, b.Stderr = os.Stderr, os.Stderr
			if err := b.Run(); err != nil {
				return "", fmt.Errorf("build: %w", err)
			}
			break
		}
	}
	bin := filepath.Join(dir, "kira-space")
	b := exec.Command("go", "build", "-tags", "server", "-o", bin, "./apps/kira-space")
	b.Stdout, b.Stderr = os.Stderr, os.Stderr
	if err := b.Run(); err != nil {
		return "", fmt.Errorf("build kira-space: %w", err)
	}
	return bin, nil
}

type progress struct {
	mu       sync.Mutex
	start    time.Time
	total    int
	done     int
	outcomes map[string]int
}

func (p *progress) add(r Result, led *runledger.Ledger) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done++
	key := r.Outcome
	if r.Outcome == outKept {
		key = "kept:" + r.Gate
	}
	p.outcomes[key]++
	if p.done%progressN == 0 || p.done == p.total {
		el := time.Since(p.start)
		eta := time.Duration(float64(el) / float64(p.done) * float64(p.total-p.done))
		fmt.Printf("done %d/%d spend %.2f USD eta %s %v\n", p.done, p.total, led.Total(), eta.Round(time.Minute), p.outcomes)
	}
}

// pendingUnits are the units with no usable result yet, one per inputKey.
func pendingUnits(o runOpts, units []Unit, results map[string]Result) []Unit {
	var todo []Unit
	seen := map[string]bool{}
	for _, u := range unitsOfLang(units, o.lang) {
		if r, ok := results[u.InputKey]; (ok && r.Outcome != outError) || seen[u.InputKey] {
			continue
		}
		if len(o.prefixes) > 0 && !hasPrefix(u.File, o.prefixes) {
			continue
		}
		seen[u.InputKey] = true
		todo = append(todo, u)
	}
	if o.limit > 0 && len(todo) > o.limit {
		todo = todo[:o.limit]
	}
	return todo
}

// queue hands units to workers in order and stops at cancellation.
type queue struct {
	mu   sync.Mutex
	next int
	todo []Unit
	ctx  context.Context
}

func (q *queue) pull() (Unit, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.next >= len(q.todo) || q.ctx.Err() != nil {
		return Unit{}, false
	}
	q.next++
	return q.todo[q.next-1], true
}

// sink appends results to results.jsonl one at a time and counts calls.
type sink struct {
	mu    sync.Mutex
	calls int
	err   error
}

// put records res; false means the run was cancelled mid-call and res is discarded.
func (s *sink) put(ctx context.Context, res Result) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if res.Outcome == outError && ctx.Err() != nil {
		return false
	}
	if err := appendWork("results.jsonl", res); err != nil && s.err == nil {
		s.err = err
	}
	return true
}

func runWorker(ctx context.Context, bin, home string, q *queue, led *runledger.Ledger, out *sink, prog *progress) {
	defer os.RemoveAll(home)
	w := &worker{bin: bin, home: home}
	defer w.reset()
	for {
		u, ok := q.pull()
		if !ok || !led.Reserve(reserveUSD) {
			return
		}
		res := processUnit(ctx, w, u, led)
		if !out.put(ctx, res) {
			return
		}
		prog.add(res, led)
	}
}

func runCmd(ctx context.Context, o runOpts) error {
	units, err := loadUnits()
	if err != nil {
		return err
	}
	if len(units) == 0 {
		return errors.New("no units; run scan first")
	}
	results, err := loadResults()
	if err != nil {
		return err
	}
	todo := pendingUnits(o, units, results)
	fmt.Printf("%d units to run, spent so far %.4f USD, cap %.2f\n", len(todo), runledger.TotalCharged(workDir), o.maxUSD)
	if len(todo) == 0 {
		return nil
	}
	bin := o.binary
	if bin == "" {
		dir, err := os.MkdirTemp("", "commentsweep-bin-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		if bin, err = buildBinary(dir); err != nil {
			return err
		}
	}
	led := runledger.New(workDir, o.maxUSD)
	prog := &progress{start: time.Now(), total: len(todo), outcomes: map[string]int{}}
	started := time.Now()
	q := &queue{todo: todo, ctx: ctx}
	out := &sink{}
	var wg sync.WaitGroup
	for range max(o.jobs, 1) {
		home, err := os.MkdirTemp("", "commentsweep-")
		if err != nil {
			return err
		}
		wg.Go(func() { runWorker(ctx, bin, home, q, led, out, prog) })
	}
	wg.Wait()
	_ = appendWork("runs.jsonl", RunLog{Start: started, End: time.Now(), Jobs: o.jobs, Calls: out.calls})
	fmt.Printf("charged total %.4f USD, %d calls this run\n", led.Total(), out.calls)
	if led.Stopped() {
		fmt.Fprintf(os.Stderr, "stopped: spend cap %.2f USD reached; rerun with a higher --max-usd to continue\n", o.maxUSD)
	}
	return out.err
}

func hasPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// processUnit makes the call, retrying a rate limit forever (charging nothing) and any other error once.
func processUnit(ctx context.Context, w *worker, u Unit, led *runledger.Ledger) Result {
	res := Result{ID: u.ID, InputKey: u.InputKey}
	var total time.Duration
	var lastErr error
	backoff := time.Minute
	for try := 1; try <= 2; try++ {
		start := time.Now()
		out, err := w.call(ctx, u.Text, u.Context)
		wall := time.Since(start)
		total += wall
		if err != nil && runledger.IsRateLimit(err) && ctx.Err() == nil {
			led.Note(runledger.SpendLine{Slug: u.ID, Try: try, WallMs: wall.Milliseconds()})
			fmt.Fprintf(os.Stderr, "  %s rate limited, waiting %s\n", u.ID, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
			}
			backoff = min(backoff*2, 15*time.Minute)
			try--
			continue
		}
		s := runledger.SpendLine{Slug: u.ID, Try: try, OK: err == nil, WallMs: wall.Milliseconds()}
		if err == nil {
			s.CostUSD, s.Charged = out.CostUSD, out.CostUSD
		} else {
			s.Charged = runledger.FailedCharge(err)
		}
		if try == 2 || err == nil || runledger.IsRefusal(err) {
			led.Settle(reserveUSD, s)
		} else {
			led.Book(s)
		}
		if err == nil {
			rw := out.Rewrite
			res.CostUSD, res.WallMs, res.Reason, res.Dropped = out.CostUSD, total.Milliseconds(), rw.Reason, rw.Dropped
			switch {
			case rw.Gate != "":
				res.Outcome, res.Gate = outKept, rw.Gate
			case !rw.Changed || rw.Text == u.Text:
				res.Outcome = outUnchanged
			default:
				res.Outcome, res.Text = outRewritten, rw.Text
			}
			return res
		}
		lastErr = err
		if runledger.IsRefusal(err) || ctx.Err() != nil {
			break
		}
		if try == 1 {
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				try = 2
			}
		}
	}
	res.Outcome, res.WallMs, res.Error = outError, total.Milliseconds(), clip(lastErr.Error(), 300)
	return res
}
