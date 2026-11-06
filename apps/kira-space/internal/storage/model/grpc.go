package model

import "fmt"

// SavedGrpcRequest is what a saved gRPC collection request *is* — deliberately
// field-identical to the request half of packages/shared/domain/grpc.ts's
// grpcRequestTabStateSchema, mirroring model.SavedRequest's own exact reasoning (collections.go).
// Not the tab state (the four UI-only fields — itemId, name, requestPane/responsePane/
// requestPaneHeight — stay out): saving them would make scrolling a pane mark a request dirty.
type SavedGrpcRequest struct {
	Target         string             `json:"target"`
	TLSMode        string             `json:"tlsMode"` // 'plaintext' | 'tls'
	CAFile         string             `json:"caFile"`
	ServerName     string             `json:"serverName"`
	DescriptorMode string             `json:"descriptorMode"` // 'reflection' | 'proto'
	ProtoPath      string             `json:"protoPath"`
	ImportPaths    []string           `json:"importPaths"`
	Service        string             `json:"service"`
	Method         string             `json:"method"`
	Message        string             `json:"message"`
	Metadata       []SavedGrpcMetaRow `json:"metadata"`
}

// SavedGrpcMetaRow is one metadata row — the same four fields as SavedHeader (name/value/enabled/
// description) with the same "enabled is builder-state-only" rule (P2 D6), deliberately not the
// same *type*: gRPC lowercases keys and has its own validity rule, and sharing a type across two
// protocols to save a few lines is the coupling P12 would then have to unpick (domain/grpc.ts's
// own comment, mirrored here). Description (P22b D6) is meaningless on this type's other use — a
// call history record's captured Header/Trailer/Metadata snapshot, which is server data, never a
// user-authored row — and is simply left empty there.
type SavedGrpcMetaRow struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

var grpcTLSModes = map[string]bool{"plaintext": true, "tls": true}
var grpcDescriptorModes = map[string]bool{"reflection": true, "proto": true}

// Validate checks what SQL cannot — mirrors SavedRequest.Validate's own posture (refused on
// write, dropped-and-logged on read).
func (r SavedGrpcRequest) Validate() error {
	if !grpcTLSModes[r.TLSMode] {
		return fmt.Errorf("model: saved grpc request: unrecognised tlsMode %q", r.TLSMode)
	}
	if !grpcDescriptorModes[r.DescriptorMode] {
		return fmt.Errorf("model: saved grpc request: unrecognised descriptorMode %q", r.DescriptorMode)
	}
	return nil
}

// The two streaming-kind values grpc_call_history.streaming holds (D11) — mirrors
// domain/grpc.ts's GRPC_STREAMING_KINDS.
const (
	GrpcStreamingUnary  = "unary"
	GrpcStreamingServer = "server"
)

// GrpcCallHistoryEntry is one grpc_call_history row's list projection — no snapshot, ever (D11's
// List never selects snapshot_json, mirroring ResponseHistoryEntry's own D4).
type GrpcCallHistoryEntry struct {
	ID            string  `json:"id"`
	ItemID        *string `json:"itemId"`
	TabID         string  `json:"tabId"`
	CalledAt      string  `json:"calledAt"`
	Target        string  `json:"target"`
	Method        string  `json:"method"`
	Streaming     string  `json:"streaming"`
	Environment   string  `json:"environment"`
	Code          int     `json:"code"`
	CodeName      string  `json:"codeName"`
	StatusMessage string  `json:"statusMessage"`
	ElapsedMs     int     `json:"elapsedMs"`
	MessageCount  int     `json:"messageCount"`
	MessageBytes  int     `json:"messageBytes"`
	StoredBytes   int     `json:"storedBytes"`
}

// GrpcCallSnapshotMessage is one stored message — D11's own per-message truncation flag.
type GrpcCallSnapshotMessage struct {
	Seq       int    `json:"seq"`
	JSON      string `json:"json"`
	WireBytes int    `json:"wireBytes"`
	OffsetMs  int64  `json:"offsetMs"`
	Truncated bool   `json:"truncated"`
}

// GrpcCallSnapshot is one entry's full stored shape — Entry rebuilt from the row's own summary
// columns (mirrors ResponseHistorySnapshot's own D4), the stage-1 request, every message this
// phase chose to keep, and D11's own storage-cap flags.
type GrpcCallSnapshot struct {
	Entry     GrpcCallHistoryEntry `json:"entry"`
	Target    string               `json:"target"`
	Method    string               `json:"method"`
	Streaming string               `json:"streaming"`
	Message   string               `json:"message"` // the stage-1 request JSON the user authored
	// RequestMessageTruncated (P18 D7): the request message was stored truncated at the 256 KiB
	// per-entry cap (repos/grpc_history.go's own maxHistoryBodyBytes reuse) — mirrors
	// ResponseHistorySnapshot's own requestBodyStorageTruncated.
	RequestMessageTruncated bool                      `json:"requestMessageTruncated"`
	Metadata                []SavedGrpcMetaRow        `json:"metadata"`
	Messages                []GrpcCallSnapshotMessage `json:"messages"`
	MessagesElided          bool                      `json:"messagesElided"`
	// MetadataElided: metadata, header and trailer were dropped to keep the row within the
	// history budget (repos/grpc_history.go).
	MetadataElided bool               `json:"metadataElided,omitempty"`
	Header         []SavedGrpcMetaRow `json:"header"`
	Trailer        []SavedGrpcMetaRow `json:"trailer"`
}

// GrpcCallHistoryRecord is Record's one argument — the bridge's own call site builds this from
// GrpcCallArgs (stage-1, never resolved) and the grpcclient.CallResult/messages the call actually
// produced.
type GrpcCallHistoryRecord struct {
	ItemID        string
	TabID         string
	EnvironmentID string
	Target        string
	Method        string
	Streaming     string
	Metadata      []SavedGrpcMetaRow
	Message       string
	Code          int
	CodeName      string
	StatusMessage string
	ElapsedMs     int
	MessageCount  int
	MessageBytes  int
	Messages      []GrpcCallSnapshotMessage
	Header        []SavedGrpcMetaRow
	Trailer       []SavedGrpcMetaRow
}

// Validate checks what SQL cannot, refusing on write (repos/tabs.go's posture).
func (r GrpcCallHistoryRecord) Validate() error {
	if r.TabID == "" {
		return fmt.Errorf("model: grpc call history: tabId is required")
	}
	if r.Method == "" {
		return fmt.Errorf("model: grpc call history: method is required")
	}
	if r.Streaming != GrpcStreamingUnary && r.Streaming != GrpcStreamingServer {
		return fmt.Errorf("model: grpc call history: unrecognised streaming kind %q", r.Streaming)
	}
	return nil
}
