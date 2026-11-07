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
	callTimeout = 15 * time.Minute
	retryDelay  = 30 * time.Second
)

type distilled struct {
	Lines      []string
	Unresolved int
	CostUSD    float64
}

type toolOut struct {
	Lines []struct {
		Text string `json:"text"`
	} `json:"lines"`
	Unresolved []json.RawMessage `json:"unresolved"`
	CostUSD    float64           `json:"costUsd"`
}

// distillOne runs one ingest_files distill call against a fresh kira-space memory-mcp process with temp homes.
func distillOne(ctx context.Context, bin, text string) (distilled, error) {
	home, err := os.MkdirTemp("", "docshistory-")
	if err != nil {
		return distilled{}, err
	}
	defer os.RemoveAll(home)
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "memory-mcp")
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "KIRA_SPACE_HOME="+filepath.Join(home, "space"))
	client := mcp.NewClient(&mcp.Implementation{Name: "docshistory", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
	if err != nil {
		return distilled{}, fmt.Errorf("connect: %w", err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ingest_files", Arguments: map[string]any{"distill": true, "text": text}})
	if err != nil {
		return distilled{}, err
	}
	if res.IsError {
		var msg []string
		for _, c := range res.Content {
			if t, ok := c.(*mcp.TextContent); ok {
				msg = append(msg, t.Text)
			}
		}
		return distilled{}, errors.New(strings.Join(msg, " "))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return distilled{}, err
	}
	var out toolOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return distilled{}, err
	}
	d := distilled{Unresolved: len(out.Unresolved), CostUSD: out.CostUSD}
	for _, l := range out.Lines {
		d.Lines = append(d.Lines, strings.TrimSpace(l.Text))
	}
	return d, nil
}

// estimate is the worst-case envelope from the pilot: 0.06 USD fixed plus 0.012 per 1,000 chars.
func estimate(chars int) float64 { return 0.06 + 0.012*float64(chars)/1000 }

type distillOpts struct {
	runs, binary string
	jobs         int
	maxUSD       float64
	retryFall    bool
	only         map[string]bool
}

func runDistill(ctx context.Context, o distillOpts, chapters []string) error {
	if err := os.MkdirAll(o.runs, 0o755); err != nil {
		return err
	}
	bin := o.binary
	if bin == "" {
		dir, err := os.MkdirTemp("", "docshistory-bin-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		bin = filepath.Join(dir, "kira-space")
		build := exec.Command("go", "build", "-tags", "server", "-o", bin, "./apps/kira-space")
		build.Stdout, build.Stderr = os.Stderr, os.Stderr
		if err := build.Run(); err != nil {
			return fmt.Errorf("build kira-space: %w", err)
		}
	}
	led := runledger.New(o.runs, o.maxUSD)
	for _, ch := range sortChapters(chapters, true) {
		if err := distillChapter(ctx, o, led, bin, ch); err != nil {
			return err
		}
		if led.Stopped() {
			fmt.Fprintf(os.Stderr, "stopped: spend cap %.2f USD reached (charged %.4f)\n", o.maxUSD, led.Total())
			break
		}
	}
	fmt.Printf("charged total %.4f USD\n", led.Total())
	return nil
}

func distillChapter(ctx context.Context, o distillOpts, led *runledger.Ledger, bin, ch string) error {
	if err := checkClean(ch); err != nil {
		return err
	}
	runFile := filepath.Join(o.runs, ch+".jsonl")
	recs, err := loadRecords(runFile)
	if err != nil {
		return err
	}
	commit := ""
	for _, r := range recs {
		commit = r.SourceCommit
		break
	}
	if commit == "" {
		if commit, err = chapterCommit(ch); err != nil {
			return err
		}
	}
	parsed, err := loadChapter(ch, commit)
	if err != nil {
		return err
	}
	var todo []Unit
	for _, u := range buildUnits(parsed) {
		if len(o.only) > 0 && !o.only[u.Slug] {
			continue
		}
		r, ok := recs[u.Slug]
		if ok && r.InputSha256 == u.Sha && (r.State == stateOK || (r.State == stateFallback && !o.retryFall)) {
			continue
		}
		todo = append(todo, u)
	}
	fmt.Printf("%s: %d units, %d to run\n", ch, len(recs), len(todo))
	var (
		wg   sync.WaitGroup
		wmu  sync.Mutex
		sem  = make(chan struct{}, max(o.jobs, 1))
		fail error
	)
	for _, u := range todo {
		est := estimate(len(u.Text))
		if !led.Reserve(est) {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(u Unit, est float64) {
			defer wg.Done()
			defer func() { <-sem }()
			rec := distillUnit(ctx, bin, u, commit, led, est)
			wmu.Lock()
			defer wmu.Unlock()
			if err := appendLine(runFile, rec); err != nil && fail == nil {
				fail = err
			}
			fmt.Printf("  %s %s lines=%d cost=%.4f %dms %s\n", u.Slug, rec.State, len(rec.Lines), rec.CostUSD, rec.WallMs, rec.Reason)
		}(u, est)
	}
	wg.Wait()
	return fail
}

func distillUnit(ctx context.Context, bin string, u Unit, commit string, led *runledger.Ledger, est float64) Record {
	rec := Record{Slug: u.Slug, ID: u.ID, Title: u.Title, Status: u.Status, InputSha256: u.Sha, InChars: len([]rune(u.Text)), SourceCommit: commit}
	var total time.Duration
	var lastErr error
	backoff := time.Minute
	for try := 1; try <= 2; try++ {
		start := time.Now()
		d, err := distillOne(ctx, bin, u.Text)
		wall := time.Since(start)
		total += wall
		if err != nil && runledger.IsRateLimit(err) && ctx.Err() == nil {
			// rate limit is not a unit failure: wait, retry same try, charge nothing
			led.Note(runledger.SpendLine{Slug: u.Slug, Try: try, WallMs: wall.Milliseconds()})
			fmt.Fprintf(os.Stderr, "  %s rate limited, waiting %s\n", u.Slug, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
			}
			backoff = min(backoff*2, 15*time.Minute)
			try--
			continue
		}
		s := runledger.SpendLine{Slug: u.Slug, Try: try, OK: err == nil, WallMs: wall.Milliseconds()}
		if err == nil {
			s.CostUSD, s.Charged = d.CostUSD, d.CostUSD
		} else {
			s.Charged = runledger.FailedCharge(err)
		}
		if try == 2 || err == nil || runledger.IsRefusal(err) {
			led.Settle(est, s)
		} else {
			led.Book(s)
		}
		if err == nil {
			rec.State, rec.Lines, rec.Unresolved, rec.CostUSD = stateOK, d.Lines, d.Unresolved, d.CostUSD
			rec.WallMs = total.Milliseconds()
			return rec
		}
		lastErr = err
		if runledger.IsRefusal(err) {
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
	rec.State, rec.Reason, rec.WallMs = stateFallback, clip(strings.Join(strings.Fields(lastErr.Error()), " "), 200), total.Milliseconds()
	return rec
}
