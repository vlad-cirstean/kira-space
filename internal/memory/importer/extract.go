package importer

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/memory"
)

const (
	maxFactsPerChunk = 40
	maxEvidenceLen   = 200
	extractTimeout   = 180 * time.Second
	extractBudget    = "0.50"
)

type extractDoc struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

type extractChunk struct {
	Index       int    `json:"index"`
	Count       int    `json:"count"`
	HeadingPath string `json:"headingPath"`
}

type extractWire struct {
	Document extractDoc   `json:"document"`
	Chunk    extractChunk `json:"chunk"`
	Context  string       `json:"context"`
	Text     string       `json:"text"`
}

// ModelAgent drives both steps through the active agent provider (memory.ResultRunner).
type ModelAgent struct {
	Runner memory.ResultRunner
	// Executable and SpaceHome start the kira-memory MCP server the finalize step uses; the server resolves
	// memory under that Space home.
	Executable string
	SpaceHome  string
	// Env is extra environment for that server (tests only).
	Env map[string]string
	// Prompts resolves the extract and finalize prompts; nil means the registry defaults.
	Prompts llmprompts.Source
}

func (a ModelAgent) prompts() llmprompts.Source {
	if a.Prompts == nil {
		return llmprompts.Defaults()
	}
	return a.Prompts
}

func (a ModelAgent) Available() error { return a.Runner.Available() }

func (a ModelAgent) Extract(ctx context.Context, in ExtractInput) (ExtractOutput, error) {
	input, err := json.Marshal(extractWire{
		Document: extractDoc{Path: in.Path, Title: in.Title},
		Chunk:    extractChunk{Index: in.Index + 1, Count: in.Count, HeadingPath: in.HeadingPath},
		Context:  in.Context, Text: in.Text,
	})
	if err != nil {
		return ExtractOutput{}, err
	}
	res, err := a.Runner.RunResult(ctx, memory.Call{
		System: llmprompts.MemoryImportExtractSystem.Text(a.prompts(), llmprompts.ExtractVars{MaxFacts: maxFactsPerChunk, MaxEvidence: maxEvidenceLen}), Input: string(input), Schema: llmprompts.MemoryImportExtractSystem.Schema(),
		Timeout: extractTimeout, Budget: extractBudget,
	})
	out := ExtractOutput{CostUSD: res.CostUSD}
	if err != nil {
		return out, err
	}
	var parsed struct{ Facts []Fact }
	if err := json.Unmarshal(res.Output, &parsed); err != nil {
		return out, a.Runner.Fail(harness.ErrOutput)
	}
	out.Facts = cleanFacts(parsed.Facts)
	return out, nil
}

// cleanFacts trims, drops empty and over-long facts, bounds evidence and caps the count.
func cleanFacts(in []Fact) []Fact {
	out := make([]Fact, 0, len(in))
	for _, f := range in {
		f.Fact = strings.TrimSpace(f.Fact)
		if n := utf8.RuneCountInString(f.Fact); n == 0 || n > memory.MaxFactLen {
			continue
		}
		f.Evidence = truncateRunes(strings.TrimSpace(f.Evidence), maxEvidenceLen)
		f.Section = strings.TrimSpace(f.Section)
		f.Unresolved = strings.TrimSpace(f.Unresolved)
		out = append(out, f)
		if len(out) == maxFactsPerChunk {
			break
		}
	}
	return out
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
