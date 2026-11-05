package bridge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/renameio/v2"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/memory"
	"github.com/kirathecat/kira-studio/internal/memory/importer"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// ChannelMemoryDistill carries a running distill's progress; the payload is MemoryDistillProgress.
const ChannelMemoryDistill = "kira:memory:distill"

const (
	maxDistillRequestID = 64
	// maxDistillImportLines is memory.MaxItems stores of 40 lines each: the most one distill returns.
	maxDistillImportLines = importer.MaxDistillChunks * 40
	distilledDir          = "distilled"
)

// MemoryDistillService is the Memory module's distill: ingest_files with distill true behind the UI. A separate
// bound service from MemoryService and MemoryImportService; it shares their prompts and engine.
type MemoryDistillService struct {
	mem     *MemoryService
	dialogs Dialogs
	events  appcore.Emitter

	// distilling admits one call at a time, app-wide, to bound spend.
	distilling sync.Mutex
}

func NewMemoryDistillService(mem *MemoryService, dialogs Dialogs, events appcore.Emitter) *MemoryDistillService {
	return &MemoryDistillService{mem: mem, dialogs: dialogs, events: events}
}

// MemoryDistillArgs is Distill's argument shape: exactly one of Text and Path. RequestID tags the
// progress events. Mode is "", facts or rewrite; rewrite takes text only.
type MemoryDistillArgs struct {
	RequestID string `json:"requestId"`
	Text      string `json:"text"`
	Path      string `json:"path"`
	Mode      string `json:"mode"`
}

// MemoryDistillResult sizes count characters (runes) and tokens (runes/3) of the input and of the
// output as Copy puts it on the clipboard (line texts joined by newlines). Unresolved lists facts
// the input leaves open. In rewrite mode Rewrite holds the outcome, Lines and Unresolved are empty
// and OutChars counts its text.
type MemoryDistillResult struct {
	Mode       string                    `json:"mode"`
	Rewrite    *importer.RewriteResult   `json:"rewrite"`
	Lines      []importer.Line           `json:"lines"`
	Unresolved []importer.UnresolvedFact `json:"unresolved"`
	CostUSD    float64                   `json:"costUsd"`
	Chunks     int                       `json:"chunks"`
	InChars    int                       `json:"inChars"`
	OutChars   int                       `json:"outChars"`
	InTokens   int                       `json:"inTokens"`
	OutTokens  int                       `json:"outTokens"`
}

// MemoryDistillProgress is the ChannelMemoryDistill payload. Stage is extract, verify, merge or rewrite; Done and Total count the
// parts, cross-check batches or the one call of that stage.
type MemoryDistillProgress struct {
	RequestID string  `json:"requestId"`
	Stage     string  `json:"stage"`
	Done      int     `json:"done"`
	Total     int     `json:"total"`
	CostUSD   float64 `json:"costUsd"`
}

// MemoryDistillLimits are the per-call bounds the UI shows and pre-checks.
type MemoryDistillLimits struct {
	MaxChars      int     `json:"maxChars"`
	MaxParts      int     `json:"maxParts"`
	SpendCapUSD   float64 `json:"spendCapUsd"`
	StoreMaxLines int     `json:"storeMaxLines"`
	// RewriteMaxChars bounds the text of a rewrite call.
	RewriteMaxChars int `json:"rewriteMaxChars"`
}

type MemoryDistillChoice struct {
	Canceled bool   `json:"canceled"`
	Path     string `json:"path"`
}

// MemoryDistillImportArgs saves the lines as a document and scans it as an import. Source names
// where the text came from; empty for pasted text.
type MemoryDistillImportArgs struct {
	Lines  []string `json:"lines"`
	Source string   `json:"source"`
}

// Limits returns the per-call bounds.
func (s *MemoryDistillService) Limits() MemoryDistillLimits {
	return MemoryDistillLimits{
		MaxChars: importer.MaxDistillRunes, MaxParts: importer.MaxDistillChunks,
		SpendCapUSD: importer.DistillSpendCapUSD, StoreMaxLines: memory.MaxItems,
		RewriteMaxChars: importer.RewriteMaxRunes,
	}
}

// Choose opens the native single-file picker.
func (s *MemoryDistillService) Choose() (MemoryDistillChoice, error) {
	path, err := s.dialogs.OpenFile(OpenFileRequest{
		Title:         i18n.T("backend.dialog.distillFile.title", nil),
		FilterName:    i18n.T("backend.dialog.documentsFilter.label", nil),
		FilterPattern: importFilter,
	})
	if err != nil {
		return MemoryDistillChoice{}, ipcerr.Internal(err.Error())
	}
	if path == "" {
		return MemoryDistillChoice{Canceled: true}, nil
	}
	return MemoryDistillChoice{Path: path}, nil
}

// Distill condenses the text or file into terse lines. It writes nothing to memory.db. Cancelling ctx
// stops new parts and the running ones; finished parts stay charged.
func (s *MemoryDistillService) Distill(ctx context.Context, args MemoryDistillArgs) (MemoryDistillResult, error) {
	if n := len(args.RequestID); n < 1 || n > maxDistillRequestID {
		return MemoryDistillResult{}, ipcerr.Contract("requestId must be 1 to %d characters", maxDistillRequestID)
	}
	if (strings.TrimSpace(args.Text) != "") == (args.Path != "") {
		return MemoryDistillResult{}, ipcerr.Contract("give exactly one of text and path")
	}
	if args.Path != "" && !filepath.IsAbs(args.Path) {
		return MemoryDistillResult{}, ipcerr.Contract("path must be absolute")
	}
	switch args.Mode {
	case "", importer.ModeFacts:
	case importer.ModeRewrite:
		if args.Path != "" {
			return MemoryDistillResult{}, ipcerr.Contract("mode %s takes text, not a path", importer.ModeRewrite)
		}
	default:
		return MemoryDistillResult{}, ipcerr.Contract("unknown mode %q", args.Mode)
	}
	if !s.distilling.TryLock() {
		return MemoryDistillResult{}, ipcerr.T(ipcerr.CodeConflict, "backend.space.memory.distill.busy.body", nil)
	}
	defer s.distilling.Unlock()

	d := importer.Distiller{Runner: s.mem.agent, Prompts: s.mem.prompts}
	res, err := d.Distill(ctx, importer.DistillRequest{
		Text: args.Text, Path: args.Path, Mode: args.Mode,
		Progress: func(p importer.DistillProgress) {
			s.events.Emit(ChannelMemoryDistill, MemoryDistillProgress{RequestID: args.RequestID, Stage: p.Stage, Done: p.Done, Total: p.Total, CostUSD: p.CostUSD})
		},
	})
	if err != nil {
		return MemoryDistillResult{}, distillErr(ctx, err)
	}
	lines, unresolved := res.Lines, res.Unresolved
	if lines == nil {
		lines = []importer.Line{}
	}
	if unresolved == nil {
		unresolved = []importer.UnresolvedFact{}
	}
	texts := make([]string, len(lines))
	for i, l := range lines {
		texts[i] = l.Text
	}
	out := utf8.RuneCountInString(strings.Join(texts, "\n"))
	if res.Rewrite != nil {
		out = utf8.RuneCountInString(res.Rewrite.Text)
	}
	return MemoryDistillResult{
		Mode: res.Mode, Rewrite: res.Rewrite,
		Lines: lines, Unresolved: unresolved, CostUSD: res.CostUSD, Chunks: res.Chunks,
		InChars: res.InRunes, OutChars: out,
		InTokens: tokensOf(res.InRunes), OutTokens: tokensOf(out),
	}, nil
}

// tokensOf is importer.EstimateTokens over a rune count.
func tokensOf(runes int) int { return (runes + 2) / 3 }

// distillReasonKeys maps an importer.DistillError reason to its catalog key.
var distillReasonKeys = map[string]string{
	importer.ReasonTooLong:      "backend.space.memory.distill.tooLong.body",
	importer.ReasonTooManyParts: "backend.space.memory.distill.tooManyParts.body",
	importer.ReasonTooManyFacts: "backend.space.memory.distill.tooManyFacts.body",
	importer.ReasonUnreadable:   "backend.space.memory.distill.unreadable.body",
	importer.ReasonSpendCap:     "backend.space.memory.distill.spendCap.body",
}

func distillErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ipcerr.T(ipcerr.CodeCancelled, "backend.space.memory.distill.cancelled.body", nil)
	}
	var de *importer.DistillError
	if errors.As(err, &de) {
		if key, ok := distillReasonKeys[de.Reason]; ok {
			return ipcerr.T(ipcerr.CodeBadRequest, key, de.Vars)
		}
	}
	return memoryErr(err)
}

// ToImport saves the lines as a Markdown document under the memory home and scans it as an import
// job (channel ui), the normal path for provenance, confirmation and retry.
func (s *MemoryDistillService) ToImport(args MemoryDistillImportArgs) (importer.Job, error) {
	if n := len(args.Lines); n < 1 || n > maxDistillImportLines {
		return importer.Job{}, ipcerr.Contract("lines must be 1 to %d", maxDistillImportLines)
	}
	var b strings.Builder
	source := strings.Join(strings.Fields(args.Source), " ")
	if source == "" {
		b.WriteString("# Distilled text\n\n")
	} else {
		fmt.Fprintf(&b, "# Distilled from %s\n\n", source)
	}
	for _, l := range args.Lines {
		if strings.TrimSpace(l) == "" || strings.ContainsAny(l, "\r\n") {
			return importer.Job{}, ipcerr.Contract("each line must be one non-empty line")
		}
		b.WriteString("- " + strings.TrimSpace(l) + "\n")
	}
	e, err := s.mem.importer()
	if err != nil {
		return importer.Job{}, memoryErr(err)
	}
	path, err := writeDistilled(b.String())
	if err != nil {
		return importer.Job{}, ipcerr.Internal(err.Error())
	}
	job, err := e.Create(importer.CreateArgs{Paths: []string{path}})
	return job, memoryErr(err)
}

func writeDistilled(doc string) (string, error) {
	dir := filepath.Join(memory.Home(), distilledDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "distilled-"+time.Now().UTC().Format("20060102-150405")+"-"+ids.New(ids.Distill)+".md")
	return path, renameio.WriteFile(path, []byte(doc), 0o600)
}
