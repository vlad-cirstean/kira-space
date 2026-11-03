package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/grpcclient"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// GrpcService is the bound service: schema discovery and a call, both run through the
// *existing* op scheduler HttpService already uses — no new scheduler, no new op log, no new
// cancel path. It is dependency-free of grpcclient's own internals beyond its public surface
// (Describe/Unary/ServerStream/Source/CallRequest), exactly as HttpService is of httpclient's.
type GrpcService struct {
	Deps appcore.Deps
}

// GrpcDescribeArgs names one descriptor source (D4) plus the scope stage 2 resolves the target and
// metadata against (D9) — collectionID/environmentID, both possibly empty (a scratch tab has no
// collection). Reload bypasses grpcclient's own cache (the UI's explicit Reload action, D4).
type GrpcDescribeArgs struct {
	DescriptorMode string                `json:"descriptorMode"`
	Target         string                `json:"target"`
	TLS            grpcclient.TLSConfig  `json:"tls"`
	Metadata       []grpcclient.MetaPair `json:"metadata"`
	ProtoPath      string                `json:"protoPath"`
	ImportPaths    []string              `json:"importPaths"`
	CollectionID   string                `json:"collectionId"`
	EnvironmentID  string                `json:"environmentId"`
	Reload         bool                  `json:"reload"`
	// OpID is optional; when set, Describe is a cancellable op (OpsService.Cancel), not persisted.
	OpID string `json:"opId"`
}

func validDescriptorMode(mode string) bool {
	return mode == string(grpcclient.SourceReflection) || mode == string(grpcclient.SourceProto)
}

// resolveGrpcSource is Describe's own stage 2 for a descriptor source: target and metadata are the
// two substitutable fields — protoPath/importPaths/caFile are picker-supplied local paths, never
// substituted (the rule for a form-data file row's path, re-handed rather than reopened).
// A .proto descriptor source resolves nothing, because Describe needs no target to reach one — a
// .proto file is read straight off disk. This short-circuit is correct only for Describe: Call
// dials a real network target regardless of which descriptor source supplies its schema, so it
// uses resolveGrpcCallSource below instead, which never skips target/metadata resolution.
// used accumulates into whatever slice the caller points at, so a single resolver's masking
// replacer covers a whole bridge method (each entry is a distinct (Name, Placeholder)
// pair now, not a name→value map — a resolver's own Used() already dedupes within itself, and this
// function's only caller ever calls it once per resolver, so no further dedupe is needed here).
func (s *GrpcService) resolveGrpcSource(args GrpcDescribeArgs, used *[]apivars.UsedSecret) (grpcclient.Source, error) {
	src := grpcclient.Source{
		Mode: grpcclient.SourceMode(args.DescriptorMode), Target: args.Target, TLS: args.TLS,
		Metadata: args.Metadata, ProtoPath: args.ProtoPath, ImportPaths: args.ImportPaths,
	}
	if args.DescriptorMode != string(grpcclient.SourceReflection) {
		return src, nil
	}
	if !grpcHasAnyReference(args.Target, args.Metadata, "") {
		return src, nil
	}
	resolver, err := s.Deps.ApiVars.NewResolver(args.CollectionID, args.EnvironmentID)
	if err != nil {
		return grpcclient.Source{}, err
	}
	if !resolver.Any() {
		return src, nil
	}
	src.Target = resolver.Text(args.Target)
	src.Metadata = resolveMetaPairs(resolver, args.Metadata)
	if err := resolver.Err(); err != nil {
		return grpcclient.Source{}, err
	}
	*used = append(*used, resolver.Used()...)
	return src, nil
}

// grpcHasAnyReference mirrors apivars' own referencedFields short-circuit (resolve.go) for
// gRPC's own three substitutable fields — before ever querying a secret, is there anything to
// resolve at all.
func grpcHasAnyReference(target string, metadata []grpcclient.MetaPair, message string) bool {
	if len(apivars.Names(target)) > 0 {
		return true
	}
	for _, m := range metadata {
		if len(apivars.Names(m.Name)) > 0 || len(apivars.Names(m.Value)) > 0 {
			return true
		}
	}
	return len(apivars.Names(message)) > 0
}

func resolveMetaPairs(resolver *apivars.Resolver, pairs []grpcclient.MetaPair) []grpcclient.MetaPair {
	out := make([]grpcclient.MetaPair, len(pairs))
	for i, m := range pairs {
		out[i] = grpcclient.MetaPair{Name: resolver.Text(m.Name), Value: resolver.Text(m.Value)}
	}
	return out
}

// resolveGrpcCallSource is Call's own stage 2: unlike Describe, a call always needs its target and
// metadata resolved regardless of descriptor mode (a .proto file only supplies the schema — the
// network target it calls is real either way), so there is no descriptor-mode short-circuit here.
// It also resolves the request message in the same pass, through the same Resolver instance
// (built at most once, only when there is anything at all to resolve), rather than Call building a
// second one just for the message.
func (s *GrpcService) resolveGrpcCallSource(args GrpcCallArgs) (grpcclient.Source, string, []apivars.UsedSecret, error) {
	src := grpcclient.Source{
		Mode: grpcclient.SourceMode(args.DescriptorMode), Target: args.Target, TLS: args.TLS,
		Metadata: args.Metadata, ProtoPath: args.ProtoPath, ImportPaths: args.ImportPaths,
	}
	var used []apivars.UsedSecret
	if !grpcHasAnyReference(args.Target, args.Metadata, args.MessageJSON) {
		return src, args.MessageJSON, used, nil
	}
	resolver, err := s.Deps.ApiVars.NewResolver(args.CollectionID, args.EnvironmentID)
	if err != nil {
		return grpcclient.Source{}, "", nil, err
	}
	if !resolver.Any() {
		return src, args.MessageJSON, used, nil
	}
	src.Target = resolver.Text(args.Target)
	src.Metadata = resolveMetaPairs(resolver, args.Metadata)
	resolvedMessage := resolver.Text(args.MessageJSON)
	if err := resolver.Err(); err != nil {
		return grpcclient.Source{}, "", nil, err
	}
	used = make([]apivars.UsedSecret, 0, len(resolver.Used()))
	used = append(used, resolver.Used()...)
	return src, resolvedMessage, used, nil
}

// Describe resolves a target's services and methods (D4). §0.3/D10: the reflection call's own
// metadata is resolved through the same Resolver, in this one bridge method, and never persisted —
// a Describe result records no history row.
func (s *GrpcService) Describe(ctx context.Context, args GrpcDescribeArgs) (grpcclient.Schema, error) {
	if !validDescriptorMode(args.DescriptorMode) {
		return grpcclient.Schema{}, ipcerr.Contract("descriptorMode must be 'reflection' or 'proto'")
	}
	if args.DescriptorMode == string(grpcclient.SourceReflection) && args.Target == "" {
		return grpcclient.Schema{}, ipcerr.Required("target")
	}
	if args.DescriptorMode == string(grpcclient.SourceProto) && args.ProtoPath == "" {
		return grpcclient.Schema{}, ipcerr.Required("protoPath")
	}

	var used []apivars.UsedSecret
	src, err := s.resolveGrpcSource(args, &used)
	if err != nil {
		return grpcclient.Schema{}, mapGrpcError(err)
	}
	if args.Reload {
		grpcclient.InvalidateCache(src)
	}

	var schema grpcclient.Schema
	if args.OpID == "" {
		schema, err = grpcclient.Describe(ctx, src)
	} else {
		// Cancellable, not RunOp: a describe is no operation worth a row in the Operations panel.
		runCtx, release, regErr := s.Deps.Router.Host().Cancellable(ctx, args.OpID)
		if regErr != nil {
			return grpcclient.Schema{}, mapGrpcError(regErr)
		}
		defer release()
		schema, err = grpcclient.Describe(runCtx, src)
	}
	if err != nil {
		maskGrpcError(err, used)
		return grpcclient.Schema{}, mapGrpcError(err)
	}
	return schema, nil
}

// GrpcCallArgs carries HttpSendArgs' own op-log addressing (OpID/TabID/CollectionID/
// EnvironmentID/ItemID) plus a gRPC call's own fields. EmitTo needs WindowKey to aim a streaming
// call's message batches at the one window that opened it, exactly as TabsService.List/Save and
// WindowsService.Ensure already carry it (control.ts:334-340). Streaming names which of
// Unary/ServerStream this call is — the renderer already knows from the schema it resolved via
// Describe (the method picker), so the bridge is told rather than re-resolving the schema a
// second time to answer its own question.
type GrpcCallArgs struct {
	OpID           string                `json:"opId"`
	TabID          string                `json:"tabId"`
	WindowKey      string                `json:"windowKey"`
	Streaming      bool                  `json:"streaming"`
	DescriptorMode string                `json:"descriptorMode"`
	Target         string                `json:"target"`
	TLS            grpcclient.TLSConfig  `json:"tls"`
	ProtoPath      string                `json:"protoPath"`
	ImportPaths    []string              `json:"importPaths"`
	Service        string                `json:"service"`
	Method         string                `json:"method"`
	MessageJSON    string                `json:"messageJson"`
	Metadata       []grpcclient.MetaPair `json:"metadata"`
	CollectionID   string                `json:"collectionId"`
	EnvironmentID  string                `json:"environmentId"`
	ItemID         string                `json:"itemId"`
	// Incognito is P71 §4: HttpSendArgs' own addition, mirrored here — suppresses
	// recordGrpcHistory below and rides along on the op spec so the op log skips persisting this
	// op too (adapterhost/host.go, oplog/wire.go).
	Incognito bool `json:"incognito"`
	// TimeoutMs bounds a unary call, clamped to 0..3_600_000; absent inherits api.requestTimeoutMs.
	// Zero means no deadline (the 5 s dial bound still applies).
	TimeoutMs int `json:"timeoutMs,omitempty"`
}

// Call is bridge/http.go's Send with a different payload, deliberately down to the ordering (D7):
// op.SetCommand receives the UNRESOLVED target and method, both before and after the call — the
// same P5 D6/F3 rule HttpService.Send follows, verbatim.
func (s *GrpcService) Call(ctx context.Context, args GrpcCallArgs) (grpcclient.CallResult, error) {
	if args.OpID == "" {
		return grpcclient.CallResult{}, ipcerr.Required("opId")
	}
	if args.TabID == "" {
		return grpcclient.CallResult{}, ipcerr.Required("tabId")
	}
	if !validDescriptorMode(args.DescriptorMode) {
		return grpcclient.CallResult{}, ipcerr.Contract("descriptorMode must be 'reflection' or 'proto'")
	}
	if args.Service == "" || args.Method == "" {
		return grpcclient.CallResult{}, ipcerr.Contract("service and method are required")
	}

	timeoutMs := args.TimeoutMs
	if timeoutMs == 0 {
		settings, err := s.Deps.Repos.Settings.GetAll()
		if err != nil {
			return grpcclient.CallResult{}, ipcerr.InternalErr(err)
		}
		timeoutMs = settings.Api.RequestTimeoutMs
	}
	timeoutMs = min(max(timeoutMs, 0), 3_600_000)

	tabID := args.TabID
	spec := adapterhost.OpSpec{ConnectionID: nil, Kind: "grpc", OpID: args.OpID, TabID: &tabID, Incognito: args.Incognito}
	unresolvedMethod := args.Service + "/" + args.Method

	_, value, err := s.Deps.Router.Host().RunOp(ctx, spec,
		func(runCtx context.Context, op *adapters.OpCtx) (any, error) {
			// P5 D6/F3, applied to gRPC verbatim: op_log.command is a persisted SQLite column
			// rendered in the Operations panel, so both the target and the metadata a
			// {{secret}}-shaped authorization value might live in must never reach it resolved.
			op.SetCommand(fmt.Sprintf("%s → %s", unresolvedMethod, args.Target))

			src, resolvedMessage, used, resolveErr := s.resolveGrpcCallSource(args)
			if resolveErr != nil {
				return nil, resolveErr
			}

			callReq := grpcclient.CallRequest{
				Target: src.Target, TLS: args.TLS, Source: src,
				FullMethod:  "/" + args.Service + "/" + args.Method,
				MessageJSON: resolvedMessage, Metadata: src.Metadata,
				Timeout: time.Duration(timeoutMs) * time.Millisecond,
			}

			var result grpcclient.CallResult
			var callErr error
			if args.Streaming {
				// runServerStream masks its own terminal error before emitting it to the
				// renderer (D8's push channel) — masking here too, after the fact, would be too
				// late for that already-emitted event (finding 4).
				result, callErr = s.runServerStream(runCtx, args, callReq, used)
			} else {
				result, callErr = grpcclient.Unary(runCtx, callReq)
				if callErr != nil {
					maskGrpcError(callErr, used)
				} else {
					// A *successful* call's own Header/Trailer/StatusMessage can echo a
					// substituted secret (a gateway reflecting x-request-id, an echo service, a
					// status message quoting the credential it rejected on a transport-level-OK
					// call) exactly as an error's can — maskGrpcError already covers the error
					// path, so the success path must mask before either the renderer or
					// recordGrpcHistory (which persists to kira.db) ever sees it.
					maskGrpcResult(&result, used)
				}
			}
			if callErr != nil {
				// D11: "a completed call" includes a cancellation or a failure that received
				// messages — the terminal status the caller sees either way (F8). Only a call
				// that never produced any terminal outcome at all (no Partial) records nothing.
				var gerr *grpcclient.Error
				if errors.As(callErr, &gerr) && gerr.Partial != nil {
					op.SetCommand(fmt.Sprintf("%s → %s → %s", unresolvedMethod, args.Target, gerr.Partial.CodeName))
					// Finding 6: a streaming call already recorded this row from inside
					// runServerStream, before its own terminal event went out — recording again
					// here would insert the row twice.
					if !args.Streaming {
						s.recordGrpcHistory(args, *gerr.Partial)
					}
				}
				return nil, callErr
			}

			op.SetCommand(fmt.Sprintf("%s → %s → %s", unresolvedMethod, args.Target, result.CodeName))

			// Finding 6: same as above — a streaming call's own recordGrpcHistory already ran
			// inside runServerStream before its terminal event was emitted.
			if !args.Streaming {
				s.recordGrpcHistory(args, result)
			}

			return result, nil
		})
	if err != nil {
		return grpcclient.CallResult{}, mapGrpcError(err)
	}
	result, _ := value.(grpcclient.CallResult)
	return result, nil
}

// recordGrpcHistory is the rule verbatim, applied to gRPC: recorded from args (stage 1 —
// the target/method/metadata/message as the user typed them, a secret still spelled {{name}}),
// never from the resolved values — best-effort, exactly as bridge/http.go's own Record call is: a
// failed insert logs and the call still returns its result. Only a completed call is ever recorded
// — a cancellation that received messages is still a completed call, which is why
// streaming.go's coalescer's `finish` always carries the true partial count for that path too.
//
// A unary call is recorded here, from Call's own RunOp closure,
// once grpcclient.Unary has returned — the renderer can only learn the outcome by awaiting Call's
// own promise, so by construction it never observes a state where the call finished but its
// history row does not exist yet. A *streaming* call is recorded from inside runServerStream
// instead, before its own terminal event goes out over the push channel: the renderer there is
// woken by that event rather than by Call's return, so recording after runServerStream returns
// (i.e. from here) would leave the same window open recordGrpcHistory's very existence is meant to
// close. Call itself skips recording for a streaming call (`if !args.Streaming`) precisely because
// runServerStream already did it.
func (s *GrpcService) recordGrpcHistory(args GrpcCallArgs, result grpcclient.CallResult) {
	// P71 §4.2: one guard covers all four call sites (unary and streaming) — nothing about an
	// incognito call reaches SQLite.
	if args.Incognito {
		return
	}
	streaming := model.GrpcStreamingUnary
	if args.Streaming {
		streaming = model.GrpcStreamingServer
	}
	messages := make([]model.GrpcCallSnapshotMessage, len(result.Messages))
	for i, m := range result.Messages {
		messages[i] = model.GrpcCallSnapshotMessage{Seq: m.Seq, JSON: m.JSON, WireBytes: m.WireBytes, OffsetMs: m.OffsetMs}
	}
	if err := s.Deps.Repos.GrpcHistory.Record(model.GrpcCallHistoryRecord{
		ItemID: args.ItemID, TabID: args.TabID, EnvironmentID: args.EnvironmentID,
		Target: args.Target, Method: args.Service + "/" + args.Method, Streaming: streaming,
		Metadata: metaPairsToSavedRows(args.Metadata), Message: args.MessageJSON,
		Code: int(result.Code), CodeName: result.CodeName, StatusMessage: result.StatusMessage,
		ElapsedMs: int(result.ElapsedMs), MessageCount: result.MessageCount, MessageBytes: result.MessageBytes,
		Messages: messages,
		Header:   metaPairsToSavedRows(result.Header), Trailer: metaPairsToSavedRows(result.Trailer),
	}); err != nil {
		slog.Warn("recording grpc call history failed", "scope", "bridge/grpc", "opId", args.OpID, "err", err)
	}
}

func metaPairsToSavedRows(pairs []grpcclient.MetaPair) []model.SavedGrpcMetaRow {
	out := make([]model.SavedGrpcMetaRow, len(pairs))
	for i, p := range pairs {
		out[i] = model.SavedGrpcMetaRow{Name: p.Name, Value: p.Value, Enabled: true}
	}
	return out
}

// ---- D8: the coalescing push channel ----

const (
	grpcCoalesceInterval = 60 * time.Millisecond
	grpcCoalesceMaxBatch = 64
	// grpcCoalesceMaxBytes bounds one batch's JSON so a stream of large messages (up to 16 MiB
	// each) flushes per message instead of piling 64 into one Wails event.
	grpcCoalesceMaxBytes = 4 << 20
)

// GrpcCallEvent is ChannelGrpcCall's own payload (packages/shared/domain/grpc.ts's GrpcCallEvent,
// field for field) — one coalesced batch of a server-streaming call's messages. seq is the index
// of the first message in the batch, so the renderer appends by index and a future reviewer can
// detect a gap; status/error are set only on the terminal event.
type GrpcCallEvent struct {
	CallID   string                 `json:"callId"`
	Seq      int                    `json:"seq"`
	Messages []grpcclient.Message   `json:"messages"`
	Done     bool                   `json:"done"`
	Status   *grpcclient.CallResult `json:"status,omitempty"`
	Error    *GrpcCallEventErr      `json:"error,omitempty"`
}

type GrpcCallEventErr struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// grpcCoalescerFinal is the extra payload only the terminal flush carries.
type grpcCoalescerFinal struct {
	status  *grpcclient.CallResult
	errInfo *GrpcCallEventErr
}

// grpcCoalescer is the buffer: wraps appevent.Coalescer[grpcclient.Message, grpcCoalescerFinal]
// accumulating messages from the goroutine RecvMsg-ing the stream and flushing on
// whichever comes first — 60 ms elapsed, 64 messages, or a terminal event (flushed immediately,
// unconditionally, even with zero pending messages, so the renderer always sees a done:true
// event). It also tracks nextSeq — the running index of the first message in each batch — which
// the generic Coalescer knows nothing about, since only this and searchCoalescer need it.
type grpcCoalescer struct {
	emit      appcore.Emitter
	windowKey string
	callID    string
	nextSeq   int
	gen       *appevent.Coalescer[grpcclient.Message, grpcCoalescerFinal]
}

// grpcMessageWeight counts a message by its JSON size, floored so grpcCoalesceMaxBatch small
// messages still fill a batch.
func grpcMessageWeight(m grpcclient.Message) int {
	return max(len(m.JSON), grpcCoalesceMaxBytes/grpcCoalesceMaxBatch)
}

func newGrpcCoalescer(emit appcore.Emitter, windowKey, callID string) *grpcCoalescer {
	c := &grpcCoalescer{emit: emit, windowKey: windowKey, callID: callID}
	c.gen = appevent.NewCoalescer(grpcCoalesceInterval, grpcCoalesceMaxBytes, grpcMessageWeight, c.flush)
	return c
}

// push is grpcclient.ServerStream's onMessage callback — called synchronously from the one
// goroutine reading the stream, never concurrently with itself.
func (c *grpcCoalescer) push(m grpcclient.Message) { c.gen.Push(m) }

// finish is the terminal flush — always sent, even with nothing pending, so the renderer always
// learns the call ended.
func (c *grpcCoalescer) finish(status *grpcclient.CallResult, errInfo *GrpcCallEventErr) {
	c.gen.Finish(grpcCoalescerFinal{status: status, errInfo: errInfo})
}

// flush runs serialized under the generic Coalescer's own lock — the sole mutator of nextSeq, so
// no lock of its own is needed for it.
func (c *grpcCoalescer) flush(msgs []grpcclient.Message, done bool, final grpcCoalescerFinal) {
	seq := c.nextSeq
	c.nextSeq += len(msgs)
	c.emit.EmitTo(c.windowKey, ChannelGrpcCall, GrpcCallEvent{
		CallID: c.callID, Seq: seq, Messages: msgs, Done: done, Status: final.status, Error: final.errInfo,
	})
}

// serverStreamFn — grpcclient.ServerStream, the exact function runServerStream calls — is a
// package-level var rather than a direct call so a test can swap in a fake that returns a
// controlled CallResult/error with no real network round trip (httpclient/wire.go's own
// wireProxyFunc is this codebase's existing precedent for the same seam-by-var shape).
var serverStreamFn = grpcclient.ServerStream

// runServerStream is D8's own call: one op stays open for the life of the stream (P2 F12: a
// long-held bound call does not block the control plane), pushing every message through the
// coalescer above, and returning the terminal CallResult so a caller that misses every event still
// ends in a correct final state.
//
// Finding 4: a failed call's error must be masked *before* coalescer.finish emits it — Call's own
// masking, applied to the returned error after this function returns, runs too late for an event
// that has already gone out over the D8 push channel (and the renderer prefers that event's error
// over Call's own return value). used is threaded in so masking can happen here, on the same
// *grpcclient.Error the coalescer is about to emit. The Partial handed to EmitTo is a copy, taken
// after masking, so nothing downstream (Call's own recordGrpcHistory read of the same error) can
// alias the struct the coalescer has already queued for emission.
//
// P21 round 2 functional finding 6: for the same reason, recordGrpcHistory itself must run
// *before* coalescer.finish, not after this function returns back into Call. For a unary call the
// renderer only learns the outcome by awaiting Call's own promise, so bridge/http.go's own
// ordering (Record inside the send closure, before the send resolves) is automatically preserved.
// A *streaming* call's renderer is instead woken by the terminal event finish emits — if that
// event goes out before the row exists in grpc_call_history, a History pane already open for this
// tab can react to the event, immediately re-list, and be answered before the insert lands: the
// finished call's own row is then simply missing from the list (and the pane's staleness flag
// incorrectly cleared) until some unrelated later call, delete or clear happens to trigger another
// refresh. Recording here, before finish, closes that window the same way http.go's ordering
// already does for its one code path. Call itself no longer records for a streaming call (see its
// own `if !args.Streaming` guards) — recording twice would double-insert the row.
func (s *GrpcService) runServerStream(ctx context.Context, args GrpcCallArgs, req grpcclient.CallRequest, used []apivars.UsedSecret) (grpcclient.CallResult, error) {
	coalescer := newGrpcCoalescer(s.Deps.Events, args.WindowKey, args.OpID)

	result, err := serverStreamFn(ctx, req, coalescer.push)
	if err != nil {
		maskGrpcError(err, used)
		var gerr *grpcclient.Error
		switch {
		case errors.As(err, &gerr) && gerr.Partial != nil:
			partial := *gerr.Partial
			// D11: a cancellation or failure that received messages is still a completed call —
			// recorded here, before the terminal event that could race a History pane's own
			// refetch of this same row.
			s.recordGrpcHistory(args, partial)
			coalescer.finish(&partial, &GrpcCallEventErr{Code: gerr.Code, Message: gerr.Message})
		case errors.As(err, &gerr):
			coalescer.finish(nil, &GrpcCallEventErr{Code: gerr.Code, Message: gerr.Message})
		default:
			coalescer.finish(nil, &GrpcCallEventErr{Code: grpcclient.CodeTransport, Message: err.Error()})
		}
		return grpcclient.CallResult{}, err
	}
	// Same reasoning as Call's own unary success branch: a successful stream's terminal
	// Header/Trailer/StatusMessage must be masked before coalescer.finish emits it over the push
	// channel — after finish returns is too late, the same "too-late-to-mask-afterwards" problem
	// already identified for the error case.
	maskGrpcResult(&result, used)
	s.recordGrpcHistory(args, result)
	coalescer.finish(&result, nil)
	return result, nil
}

// ---- D10/D16: masking and error mapping ----

// maskGrpcError masks a *grpcclient.Error's own Message (and, for a partial stream result, its
// StatusMessage/Header/Trailer values) back to {{name}} form before it ever reaches mapGrpcError —
// the same strings.Replacer shape bridge/http.go's secretReplacer builds, a second consumer of the
// one unexported helper this package already has. A no-op for any error that
// is not a *grpcclient.Error, or when nothing was substituted.
func maskGrpcError(err error, used []apivars.UsedSecret) {
	var gerr *grpcclient.Error
	if !errors.As(err, &gerr) {
		return
	}
	replacer := secretReplacer(used)
	if replacer == nil {
		return
	}
	gerr.Message = replacer.Replace(gerr.Message)
	if gerr.Partial != nil {
		gerr.Partial.StatusMessage = replacer.Replace(gerr.Partial.StatusMessage)
		for i := range gerr.Partial.Header {
			gerr.Partial.Header[i].Value = replacer.Replace(gerr.Partial.Header[i].Value)
		}
		for i := range gerr.Partial.Trailer {
			gerr.Partial.Trailer[i].Value = replacer.Replace(gerr.Partial.Trailer[i].Value)
		}
	}
}

// maskGrpcResult is maskGrpcError's own success-path sibling: a *grpcclient.CallResult's
// StatusMessage/Header/Trailer come straight from the server on a call that returned no error, and
// can echo a substituted secret exactly as an error's own fields can (a gateway reflecting a
// metadata value, a status message quoting the credential it rejected on an otherwise-OK
// transport). bridge/http.go's own maskSecrets already does this for HTTP's successful response;
// gRPC's success path had no equivalent. A no-op when nothing was substituted,
// same as maskGrpcError.
func maskGrpcResult(result *grpcclient.CallResult, used []apivars.UsedSecret) {
	replacer := secretReplacer(used)
	if replacer == nil {
		return
	}
	result.StatusMessage = replacer.Replace(result.StatusMessage)
	for i := range result.Header {
		result.Header[i].Value = replacer.Replace(result.Header[i].Value)
	}
	for i := range result.Trailer {
		result.Trailer[i].Value = replacer.Replace(result.Trailer[i].Value)
	}
}

// mapGrpcError joins grpcclient's own four-code vocabulary into the ipcerr family — mirrors
// mapHttpError (bridge/http.go): when the failure carries a Partial CallResult (a stream that
// delivered some messages before failing or being cancelled), it is marshalled into Details
// for a renderer that knows how to read it, the same channel mapHttpError already uses.
func mapGrpcError(err error) error {
	var gerr *grpcclient.Error
	if errors.As(err, &gerr) {
		e := ipcerr.New(gerr.Code, gerr.Message)
		if gerr.Partial != nil {
			if b, mErr := json.Marshal(gerr.Partial); mErr == nil {
				e.Details = b
			} else {
				slog.Warn("marshalling partial gRPC call result failed", "scope", "bridge/grpc", "err", mErr)
			}
		}
		return e
	}
	return ipcerr.Wrap(err)
}
