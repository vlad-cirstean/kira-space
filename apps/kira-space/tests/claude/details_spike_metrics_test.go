package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// spikeEditStats are the edit-ease and read-effort counters of one run, read from its stream.
type spikeEditStats struct {
	EditCalls     int // Edit, MultiEdit and Write tool calls
	ShellWrites   int // Bash commands that write (redirect, tee, cp, mv, sed -i)
	FailedEdits   int // edit-path calls that failed: edit/write/shell-write errors, kira tool errors, submit or finish results invalid/rejected
	Retried       int // a failed edit-path call followed by another call on the same file or tool
	WholeRewrites int // Write over an existing file
	WrongFile     int // write to a read-only task file (README.md, task.yaml, state.yaml of the task folder)
	ReadCalls     int // Read, Glob, Grep and read-only Bash (cat, ls, head, find, grep, ...)
	ReadBytes     int // bytes of their results
	DocBytes      int // result bytes of reads that touched README.md or AGENTS.md
	HistReads     int // read calls that touched handovers or notes
	HistBytes     int // their result bytes
	SearchCalls   int // ToolSearch calls
	SearchCtx     int // context growth caused by ToolSearch results
}

var (
	shellWriteRe = regexp.MustCompile(`(>|\btee\b|\bcp\b|\bmv\b|sed\s+-i)`)
	shellNoiseRe = regexp.MustCompile(`[0-9]*>&[0-9]|[0-9]*>\s*/dev/null`)
	shellReadRe  = regexp.MustCompile(`\b(cat|ls|head|tail|find|grep|wc|stat)\b`)
	docRe        = regexp.MustCompile(`(README|AGENTS)\.md`)
	histRe       = regexp.MustCompile(`handovers|notes`)
	readOnlyRe   = regexp.MustCompile(`/tasks/t-1/(README\.md|task\.yaml|state\.yaml)$`)
)

func isShellWrite(cmd string) bool {
	return shellWriteRe.MatchString(shellNoiseRe.ReplaceAllString(cmd, ""))
}

// parseSpikeStream fills rec from a stream-json transcript (tool counts, context per call, edit
// stats) and returns the result line.
func parseSpikeStream(raw string, rec *spikeRec) spikeResultLine {
	if rec.Tools == nil {
		rec.Tools = map[string]int{}
	}
	var res spikeResultLine
	var ids []string
	var ctxs []int
	type use struct {
		name, file, cmd string
		write, read     bool
		shell           bool
	}
	uses := map[string]use{}
	var order []string // tool_use ids in call order
	var searchAt []int // ctx index of each model call that issued ToolSearch
	failedFile := map[string]bool{}
	var lastFailed *use
	for _, line := range strings.Split(raw, "\n") {
		var head struct {
			Type    string `json:"type"`
			Subtype string `json:"subtype"`
			Message struct {
				ID      string          `json:"id"`
				Content json.RawMessage `json:"content"`
				Usage   struct {
					In         int `json:"input_tokens"`
					CacheRead  int `json:"cache_read_input_tokens"`
					CacheWrite int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &head) != nil {
			continue
		}
		switch head.Type {
		case "system":
			if head.Subtype == "init" {
				var in spikeResultLine
				_ = json.Unmarshal([]byte(line), &in)
				for _, n := range in.Tools {
					if strings.HasPrefix(n, "mcp__") {
						rec.MCPTools++
					}
				}
			}
		case "assistant":
			ctxN := head.Message.Usage.In + head.Message.Usage.CacheRead + head.Message.Usage.CacheWrite
			if id := head.Message.ID; id != "" && ctxN > 0 {
				if n := len(ctxs); n > 0 && ids[n-1] == id {
					ctxs[n-1] = ctxN
				} else {
					ids, ctxs = append(ids, id), append(ctxs, ctxN)
				}
			}
			var bs []contentBlock
			_ = json.Unmarshal(head.Message.Content, &bs)
			for _, b := range bs {
				if b.Type != "tool_use" {
					continue
				}
				if b.Name == "ToolSearch" {
					rec.SearchCalls++
					searchAt = append(searchAt, len(ctxs)-1)
				}
				short := strings.TrimPrefix(strings.TrimPrefix(b.Name, "mcp__kira-space__"), "mcp__kira-studio__")
				rec.Tools[short]++
				var in struct {
					FilePath string `json:"file_path"`
					Command  string `json:"command"`
				}
				_ = json.Unmarshal(b.Input, &in)
				u := use{name: b.Name, file: in.FilePath, cmd: in.Command}
				switch {
				case b.Name == "Edit" || b.Name == "MultiEdit" || b.Name == "Write":
					u.write = true
					rec.EditCalls++
					if readOnlyRe.MatchString(in.FilePath) {
						rec.WrongFile++
					}
				case b.Name == "Bash" && isShellWrite(in.Command):
					u.write, u.shell = true, true
					rec.ShellWrites++
					for _, m := range readOnlyRe.FindAllString(in.Command, -1) {
						_ = m
						rec.WrongFile++
						break
					}
				case b.Name == "Read" || b.Name == "Glob" || b.Name == "Grep" || (b.Name == "Bash" && shellReadRe.MatchString(in.Command)):
					u.read = true
					rec.ReadCalls++
				case strings.HasPrefix(b.Name, "mcp__kira-"):
					u.write = true // a kira tool call mutates or queries Kira state; its errors count as failed edit-path calls
				}
				uses[b.ID] = u
				order = append(order, b.ID)
				// a call after a failed one on the same file or tool is a retry
				if lastFailed != nil && ((u.file != "" && u.file == lastFailed.file) || u.name == lastFailed.name) {
					rec.Retried++
				}
				lastFailed = nil
			}
		case "user":
			var bs []contentBlock
			_ = json.Unmarshal(head.Message.Content, &bs)
			for _, b := range bs {
				if b.Type != "tool_result" {
					continue
				}
				u, ok := uses[b.ToolUseID]
				if !ok {
					continue
				}
				txt := toolResultText(b)
				if u.read {
					rec.ReadBytes += len(txt)
					where := u.file + " " + u.cmd
					if docRe.MatchString(where) {
						rec.DocBytes += len(txt)
					}
					if histRe.MatchString(where) {
						rec.HistReads++
						rec.HistBytes += len(txt)
					}
				}
				bad := b.IsError
				if !bad && (u.name == "mcp__kira-space__submit" || u.name == "mcp__kira-space__finish_step") {
					bad = strings.Contains(txt, `"result":"invalid"`) || strings.Contains(txt, `"result":"rejected"`) || strings.Contains(txt, `"result":"restored"`)
				}
				if u.name == "Write" && strings.Contains(txt, "has been updated") {
					rec.WholeRewrites++
				}
				if bad && u.write {
					rec.FailedEdits++
					uu := u
					lastFailed = &uu
					failedFile[u.file] = true
				}
			}
		case "result":
			_ = json.Unmarshal([]byte(line), &res)
		}
	}
	_ = order
	for _, i := range searchAt {
		if i >= 0 && i+1 < len(ctxs) {
			rec.SearchCtx += ctxs[i+1] - ctxs[i]
		}
	}
	if n := len(ctxs); n > 0 {
		rec.CtxFirst, rec.CtxFinal = ctxs[0], ctxs[n-1]
		if n > 1 {
			rec.CtxGrowth = float64(ctxs[n-1]-ctxs[0]) / float64(n-1)
		}
	}
	rec.Cost, rec.Turns = res.TotalCostUSD, res.NumTurns
	rec.In, rec.Out, rec.CacheRead, rec.CacheWrite = res.Usage.In, res.Usage.Out, res.Usage.CacheRead, res.Usage.CacheWrite
	return res
}

func toolResultText(b contentBlock) string {
	var s string
	if json.Unmarshal(b.Content, &s) == nil {
		return s
	}
	var parts []contentBlock
	var sb strings.Builder
	if json.Unmarshal(b.Content, &parts) == nil {
		for _, p := range parts {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

var logRunRe = regexp.MustCompile(`(F\d) ([TF]) (haiku|sonnet) #(\d+) pass=(true|false) cost=[0-9.]+ turns=\d+ ?(.*)$`)

// TestDetailsSpikeReport rebuilds the results file from the raw transcripts of an earlier
// TestDetailsSpike run (KIRA_SPIKE_DUMP dir) and its verbose log (pass flags and failure notes).
// It costs nothing: KIRA_SPIKE_REPORT_DIR=<dump dir> KIRA_SPIKE_REPORT_LOG=<log> [KIRA_SPIKE_OUT=...].
func TestDetailsSpikeReport(t *testing.T) {
	dir, logFile := os.Getenv("KIRA_SPIKE_REPORT_DIR"), os.Getenv("KIRA_SPIKE_REPORT_LOG")
	if dir == "" || logFile == "" {
		t.Skip("set KIRA_SPIKE_REPORT_DIR and KIRA_SPIKE_REPORT_LOG to rebuild the report from a past run")
	}
	pass := map[string][2]string{}
	raw, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(raw), "\n") {
		if m := logRunRe.FindStringSubmatch(l); m != nil {
			pass[strings.Join(m[1:5], "-")] = [2]string{m[5], strings.TrimSpace(m[6])}
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	var recs []spikeRec
	var spent float64
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		parts := strings.Split(base, "-")
		if len(parts) != 4 {
			continue
		}
		rep, _ := strconv.Atoi(parts[3])
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rec := spikeRec{Flow: parts[0], Arm: parts[1], Model: parts[2], Rep: rep}
		parseSpikeStream(string(b), &rec)
		p := pass[base]
		rec.Pass, rec.Why = p[0] == "true", p[1]
		spent += rec.Cost
		recs = append(recs, rec)
	}
	out := os.Getenv("KIRA_SPIKE_OUT")
	if out == "" {
		out = "../../../../docs/v0.3.14/plans/P643-spike-results.md"
	}
	if err := os.WriteFile(out, []byte(spikeReport(recs, spent, "", len(recs))), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d runs, %s", len(recs), fmt.Sprintf("%.4f USD", spent))
}
