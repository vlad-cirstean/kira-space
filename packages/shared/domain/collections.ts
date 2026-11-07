import { z } from 'zod';
import { grpcDescriptorModeSchema, grpcMetadataSchema, grpcTlsModeSchema } from './grpc';
import {
  httpBinaryFileSchema,
  httpBodyModeSchema,
  httpCodeLanguageSchema,
  httpFormDataFieldSchema,
  httpHeaderSchema,
  httpUrlEncodedFieldSchema,
} from './http';

// The TypeScript mirror of Go's model.SavedRequest — the **request half** of
// httpRequestTabStateSchema, field name for field name, and nothing else.
//
// Three things it deliberately is not:
//   - Not the tab state. The four UI-only fields (requestPane, responsePane, responseView,
//     requestPaneHeight) stay out: they are per-tab furniture, and saving them into a collection
//     would make scrolling a pane mark a request dirty.
//   - Not the wire body. httpBodyWire drops disabled rows; a saved request must keep one — that
//     is the whole point of the checkbox.
//   - Not opaque, unlike tabs.state_json. Go writes this document too (on import), so Go owns the
//     type and this mirrors it — the same division httpclient.Body/HttpBodyWire already has,
//     applied to a stored document instead of a call argument.
//
// Unlike most mirrors in this file's neighbourhood, this one **is** Zod-parsed — at exactly one
// boundary, `openCollectionRequestTab`, where the parsed result becomes tab state and a bad shape
// would break a render. That reuses TabKindDef.parseState's mechanism rather than adding a second
// trust boundary.
//
// `method` is a plain string, not httpMethodSchema's seven-member enum: Postman's own method list
// is 15 values plus any custom string, and Go stores whatever the file said. The coercion to
// a method the builder can show happens at that same one boundary, not here — rejecting the whole
// document over one field would drop a request this app can otherwise open perfectly well.
export const httpSavedRequestSchema = /*#__PURE__*/ z.object({
  method: z.string().default('GET'),
  url: z.string().default(''),
  headers: /*#__PURE__*/ z.array(httpHeaderSchema).default([]),
  bodyMode: httpBodyModeSchema.default('none'),
  body: z.string().default(''),
  code: z.string().default(''),
  codeLanguage: httpCodeLanguageSchema.default('json'),
  urlEncoded: /*#__PURE__*/ z.array(httpUrlEncodedFieldSchema).default([]),
  formData: /*#__PURE__*/ z.array(httpFormDataFieldSchema).default([]),
  binaryFile: httpBinaryFileSchema,
  // P22b D7: a query param's own description, keyed by name — see httpRequestTabStateSchema's own
  // comment (domain/http.ts) for why this is a side-car map rather than a `params` array. A plain
  // `.default({})` is not enough here, unlike every other field's `.default()` on this schema:
  // this member is brand new, so *every* row saved before this phase omits the Go struct field
  // entirely, and Go's own JSON encoder (no `omitempty` tag, matching this file's siblings) writes
  // an absent/nil map as the literal `null` rather than dropping the key — and Zod's `.default()`
  // only ever fires for `undefined`, never `null` (confirmed against this repo's own zod version).
  // The preprocess step normalizes both to `{}` before the record schema ever sees them.
  paramDescriptions: /*#__PURE__*/ z
    .preprocess((v) => v ?? {}, z.record(z.string(), z.string()))
    .default({}),
});
export type HttpSavedRequest = z.infer<typeof httpSavedRequestSchema>;

/** An empty saved request — what a brand-new collection row starts as. */
export function defaultHttpSavedRequest(): HttpSavedRequest {
  return httpSavedRequestSchema.parse({});
}

// The TypeScript mirror of Go's model.SavedGrpcRequest — the request half of
// grpcRequestTabStateSchema (domain/grpc.ts), field name for field name, following
// httpSavedRequestSchema's own precedent exactly: not the tab state (the four UI-only fields stay
// out), and Go owns the type since it writes this document too.
export const grpcSavedRequestSchema = /*#__PURE__*/ z.object({
  target: z.string().default(''),
  tlsMode: grpcTlsModeSchema.default('tls'),
  caFile: z.string().default(''),
  serverName: z.string().default(''),
  descriptorMode: grpcDescriptorModeSchema.default('reflection'),
  protoPath: z.string().default(''),
  importPaths: /*#__PURE__*/ z.array(z.string()).default([]),
  service: z.string().default(''),
  method: z.string().default(''),
  message: z.string().default(''),
  metadata: /*#__PURE__*/ z.array(grpcMetadataSchema).default([]),
});
export type GrpcSavedRequest = z.infer<typeof grpcSavedRequestSchema>;

/** An empty saved gRPC request — what a brand-new collection row starts as. */
export function defaultGrpcSavedRequest(): GrpcSavedRequest {
  return grpcSavedRequestSchema.parse({});
}

// The two flat row shapes CollectionsService.List answers with (D11). Mirrors of Go's
// model.Collection / model.CollectionItem, typed rather than validated — the renderer builds the
// tree from these, the same way TreeService.Children returns flat nodes.
export interface CollectionSummary {
  id: string;
  name: string;
  /** The collection's config file name in the api folder. */
  file: string;
  sortOrder: number;
  createdAt: string;
  updatedAt: string;
}

/** `method` and `url` are denormalized out of the saved request so the tree renders a method chip
 *  and searches URLs without reading a body; both are '' for a folder. `protocol` is
 *  structural sibling data, not a third `kind` value — `kind` stays 'folder' vs. a leaf, and
 *  `protocol` says which document shape `request_json` holds for a leaf: 'http' ->
 *  model.SavedRequest, 'grpc' -> model.SavedGrpcRequest. Meaningless for a folder, defaulted to
 *  'http' there. */
export interface CollectionItemSummary {
  id: string;
  collectionId: string;
  parentId: string | null;
  kind: CollectionItemKind;
  name: string;
  sortOrder: number;
  method: string;
  url: string;
  protocol: CollectionItemProtocol;
  createdAt: string;
  updatedAt: string;
}

export const COLLECTION_ITEM_KINDS = ['folder', 'request'] as const;
export type CollectionItemKind = (typeof COLLECTION_ITEM_KINDS)[number];

export const COLLECTION_ITEM_PROTOCOLS = ['http', 'grpc'] as const;
export type CollectionItemProtocol = (typeof COLLECTION_ITEM_PROTOCOLS)[number];

/** Which of the two tables a tree row lives in — the renderer passes this rather than knowing. */
export type CollectionTarget = 'collection' | 'item';

// P4 D12: the import report is part of the feature, not decoration. Every warning kind is a case
// where the app quietly does something other than what the file says, and the alternative to
// telling the user is letting them find out from a 401 or an E_BAD_REQUEST minutes later.
export interface ImportWarning {
  kind: string;
  count: number;
  detail: string;
}

/** One imported Postman environment file (CollectionsService.Import). */
export interface EnvironmentImportReport {
  environmentId: string;
  name: string;
  variables: number;
  warnings: ImportWarning[];
}

/** One picked file's outcome: `kind` is '' (and `error` set) when the file failed. */
export interface ImportFileResult {
  path: string;
  fileName: string;
  kind: 'collection' | 'environment' | '';
  collection: ImportReport | null;
  environment: EnvironmentImportReport | null;
  error: string | null;
}

export interface ImportResult {
  files: ImportFileResult[];
}

export interface ImportReport {
  collectionId: string;
  name: string;
  folders: number;
  requests: number;
  warnings: ImportWarning[];
}
