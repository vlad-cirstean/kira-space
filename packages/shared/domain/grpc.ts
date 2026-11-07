import { t } from '@theme/i18n';
import { z } from 'zod';

// The wire shapes live in Go (internal/grpcclient) and are mirrored, not re-validated,
// here — control.ts's grpcDescribe/grpcCall trust<T>() the bound call's result exactly as every
// other bound call does. These types exist so the renderer has something to type against, not to
// guard input Go already produced.

/** internal/grpcclient.MetaPair — one gRPC metadata name/value pair. F6: keys are lowercased by
 *  the time this reaches the renderer. */
export interface GrpcMetaPairWire {
  name: string;
  value: string;
}

/** internal/grpcclient.Method — one method the schema browser lists. */
export interface GrpcMethodWire {
  name: string;
  fullName: string;
  clientStreaming: boolean;
  serverStreaming: boolean;
  inputType: string;
  outputType: string;
  /** The empty-instance JSON of the input message (EmitUnpopulated + Multiline) — the "fill this
   *  in" template the message editor seeds with (D4). */
  requestTemplate: string;
}

/** internal/grpcclient.Service — one service the schema browser lists. */
export interface GrpcServiceWire {
  /** Short name, for display. */
  name: string;
  /** Package-qualified name; what a Call addresses. */
  fullName: string;
  methods: GrpcMethodWire[];
}

/** internal/grpcclient.Schema — GrpcService.Describe's whole answer (D4). */
export interface GrpcSchemaWire {
  services: GrpcServiceWire[];
  mode: string;
  warnings: string[];
}

/** internal/grpcclient.Message — one message of a call, arrival offset and wire size included
 *  (F5, F7). */
export interface GrpcMessageWire {
  seq: number;
  json: string;
  wireBytes: number;
  offsetMs: number;
}

/** internal/grpcclient.CallResult — D16: a non-OK gRPC status lives here, not in a thrown error.
 *  `messages` carries every message a call produced: exactly one for a unary call, N for a
 *  streaming one (which also pushes each one over ChannelGrpcCall as it arrives, D8). */
export interface GrpcCallResultWire {
  code: number;
  codeName: string;
  statusMessage: string;
  elapsedMs: number;
  header: GrpcMetaPairWire[];
  trailer: GrpcMetaPairWire[];
  messageCount: number;
  messageBytes: number;
  messages?: GrpcMessageWire[];
}

/** D8: one batch of a server-streaming call's messages, pushed over ChannelGrpcCall. `seq` is the
 *  index of the first message in the batch, so the renderer appends by index and a future
 *  reviewer can detect a gap. `done`/`status` are set only on the terminal event. */
export interface GrpcCallEvent {
  callId: string;
  seq: number;
  messages: GrpcMessageWire[];
  done: boolean;
  status?: GrpcCallResultWire;
  /** Set only on a terminal event that ended in something other than a clean stream end — e.g.
   *  Stop (F8) — so the renderer can render D17's own sentence without re-deriving it. */
  error?: { code: string; message: string };
}

// codes.Code.String() (grpc-go), verbatim — GRPC_CODE_NAMES is the renderer's own copy so a
// status chip never has to wait on a round trip to learn a code's name (Go already sends
// CallResult.codeName, but the History pane and a partial/failure path both want a name from a
// bare integer code too).
export const GRPC_CODE_NAMES: Readonly<Record<number, string>> = {
  0: 'OK',
  1: 'Canceled',
  2: 'Unknown',
  3: 'InvalidArgument',
  4: 'DeadlineExceeded',
  5: 'NotFound',
  6: 'AlreadyExists',
  7: 'PermissionDenied',
  8: 'ResourceExhausted',
  9: 'FailedPrecondition',
  10: 'Aborted',
  11: 'OutOfRange',
  12: 'Unimplemented',
  13: 'Internal',
  14: 'Unavailable',
  15: 'DataLoss',
  16: 'Unauthenticated',
};

/** The status chip's colour — OK is 'ok', a handful of "try again"-shaped codes are 'warn',
 *  everything else (including Canceled, which the app itself only ever produces via Stop) is
 *  'error'. Mirrors statusClass's own four-value vocabulary (domain/http.ts) over .p-chip's
 *  existing ok/warn/error/info variants — no new colour token. */
export function grpcCodeClass(code: number): 'info' | 'ok' | 'warn' | 'error' {
  if (code === 0) return 'ok';
  if (code === 4 || code === 8 || code === 14) return 'warn'; // DeadlineExceeded/ResourceExhausted/Unavailable
  return 'error';
}

/** The collections tree's own row needs a class for its leading chip and http/** may not import
 *  views/** (biome.json), so this lives here rather than in views/grpcrequest/. The tree's own
 *  denormalised columns (method/url, reused verbatim for a gRPC row's "pkg.Service/Method" and
 *  target) carry no streaming flag — that lives in the method's own descriptor, which a
 *  collections row cannot resolve without a live call — so every gRPC row gets the one neutral
 *  class from the status family (a gRPC call has no per-verb distinction to colour-code at all,
 *  so this stays on the plain status palette). */
export function grpcMethodClass(_method: string): 'info' | 'ok' | 'warn' | 'error' {
  return 'info';
}

// A per-code sentence, statusHint's own shape and register
// (domain/http.ts) — lowercase, no trailing period, a sentence a person can act on. Lives here,
// not in the view, for grpcCodeClass's own stated reason: http/** may not import views/**, and the
// collections tree already needs code vocabulary. No Go mirror, matching statusHint.
export const GRPC_CODE_HINTS: Readonly<Record<number, string>> = {
  0: 'the call completed',
  1: 'the call was cancelled before it completed',
  2: 'the server returned an error it could not classify more specifically',
  3: 'the server rejected the request message itself',
  4: 'the call ran past its deadline before the server answered',
  5: 'the server has no such method or resource',
  6: 'the resource the call tried to create already exists',
  7: 'the caller is authenticated but not allowed to make this call',
  8: 'a resource limit was exceeded, such as a quota or the available disk space',
  9: 'the system is not in a state required for the call to proceed',
  10: 'the call was aborted, typically due to a concurrency conflict',
  11: 'the operation was attempted past the valid range, such as seeking past the end of a file',
  12: 'the server does not implement this method',
  13: 'the server encountered an internal invariant violation',
  14: 'the server could not be reached, or is not accepting calls right now',
  15: 'unrecoverable data loss or corruption occurred',
  16: 'the call carried no valid credentials',
};

/** A per-code sentence, falling back to a generic one for a code outside the table — a code is a
 *  wire integer, and a server may send one this app does not otherwise recognise. */
export function grpcCodeHint(code: number): string {
  return GRPC_CODE_HINTS[code] ?? 'an unrecognised status code';
}

/** {name, value, enabled} — deliberately the same three fields as httpHeaderSchema
 *  (domain/http.ts) with the same "enabled is builder-state-only" rule, because it is the
 *  same idea and a person moving between the two tabs should not have to learn a second one. Not
 *  the same *type*: gRPC lowercases keys and has its own validity rule (call.go's own
 *  metadataKeyPattern). */
export const grpcMetadataSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  value: z.string(),
  enabled: z.boolean().default(true),
  // P22b D6: app-local free text, no wire counterpart — domain/http.ts's httpHeaderSchema own
  // comment, copied here for the same reason this schema already borrows its other three fields.
  description: z.string().default(''),
});
export type GrpcMetadataState = z.infer<typeof grpcMetadataSchema>;

export const grpcTlsModeSchema = /*#__PURE__*/ z.enum(['plaintext', 'tls']);
export type GrpcTlsMode = z.infer<typeof grpcTlsModeSchema>;

export const grpcDescriptorModeSchema = /*#__PURE__*/ z.enum(['reflection', 'proto']);
export type GrpcDescriptorMode = z.infer<typeof grpcDescriptorModeSchema>;

export const grpcRequestPaneSchema = /*#__PURE__*/ z.enum(['message', 'metadata', 'schema']);
export type GrpcRequestPane = z.infer<typeof grpcRequestPaneSchema>;

export const grpcResponsePaneSchema = /*#__PURE__*/ z.enum(['messages', 'metadata', 'history']);
export type GrpcResponsePane = z.infer<typeof grpcResponsePaneSchema>;

// D5: every field .default()ed, flat rather than nested, so a tab saved by an earlier build
// restores through TabKindDef.parseState's merge-only normalisation (following
// httpRequestTabStateSchema's own discipline exactly).
export const grpcRequestTabStateSchema = /*#__PURE__*/ z.object({
  target: z.string().default(''),
  tlsMode: grpcTlsModeSchema.default('tls'),
  caFile: z.string().default(''), // a path, never bytes
  serverName: z.string().default(''), // TLS SNI / authority override
  descriptorMode: grpcDescriptorModeSchema.default('reflection'),
  protoPath: z.string().default(''),
  importPaths: /*#__PURE__*/ z.array(z.string()).default([]),
  service: z.string().default(''), // fully-qualified
  method: z.string().default(''), // method name within the service
  message: z.string().default(''), // the request JSON the user authored
  metadata: /*#__PURE__*/ z.array(grpcMetadataSchema).default([]),
  itemId: z.string().nullable().default(null),
  name: z.string().default(''),
  requestPane: grpcRequestPaneSchema.default('message'),
  responsePane: grpcResponsePaneSchema.default('messages'),
  requestPaneHeight: z.number().int().min(0).default(0),
  // P22b D6/D7: MetadataTable's own description-column toggle, persisted per tab — the gRPC
  // sibling of httpRequestTabStateSchema's own fieldDescriptions (domain/http.ts).
  fieldDescriptions: z.boolean().default(false),
});
export type GrpcRequestTabState = z.infer<typeof grpcRequestTabStateSchema>;

export function defaultGrpcRequestTabState(): GrpcRequestTabState {
  return grpcRequestTabStateSchema.parse({});
}

/** D2: the saved name, else Service/Method, else the target, else 'New gRPC request' — mirrors
 *  httpRequestTitle's own precedence (views/httprequest/url.ts). */
export function grpcRequestTitle(state: {
  name: string;
  service: string;
  method: string;
  target: string;
}): string {
  if (state.name) return state.name;
  if (state.service && state.method) return `${state.service}/${state.method}`;
  if (state.target) return state.target;
  return t('shared.grpc.newRequest.title');
}

/** D12: the streaming-kind chip's own two values, mirrored from grpc_call_history.streaming
 *  (Go: model.GrpcStreamingUnary/GrpcStreamingServer). */
export const GRPC_STREAMING_KINDS = ['unary', 'server'] as const;
export type GrpcStreamingKind = (typeof GRPC_STREAMING_KINDS)[number];

export function grpcStreamingLabel(kind: GrpcStreamingKind): string {
  return kind === 'server'
    ? t('shared.grpc.streaming.stream.label')
    : t('shared.grpc.streaming.unary.label');
}
