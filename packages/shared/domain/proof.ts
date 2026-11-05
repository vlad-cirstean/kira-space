import { z } from 'zod';

// P420: the validation-proof window's domain. Mirrors internal/proof's wire types and the payload
// shapes internal/proof/validate.go stores; Go validates first, the window parses again before
// building a page from a payload.

export const proofViewSchema = /*#__PURE__*/ z.enum(['table', 'document', 'keyvalue', 'chart']);
export type ProofView = z.infer<typeof proofViewSchema>;

export const proofLanguageSchema = /*#__PURE__*/ z.enum(['sql', 'json', 'shell', 'text']);

export const proofDiffSchema = /*#__PURE__*/ z.enum(['added', 'changed', 'unchanged', 'removed']);
export type ProofDiff = z.infer<typeof proofDiffSchema>;

export const proofColumnTypeSchema = /*#__PURE__*/ z.enum([
  'number',
  'text',
  'boolean',
  'temporal',
  'json',
  'other',
]);

export const proofTablePayloadSchema = /*#__PURE__*/ z.object({
  columns: z.array(z.object({ name: z.string(), type: proofColumnTypeSchema })),
  rows: z.array(z.array(z.unknown())),
});
export type ProofTablePayload = z.infer<typeof proofTablePayloadSchema>;

export const proofDocumentPayloadSchema = /*#__PURE__*/ z.object({
  documents: z.array(z.object({ id: z.string(), body: z.record(z.string(), z.unknown()) })),
});
export type ProofDocumentPayload = z.infer<typeof proofDocumentPayloadSchema>;

export const proofKeyValuePayloadSchema = /*#__PURE__*/ z.object({
  entries: z.array(z.object({ field: z.string(), value: z.unknown() })),
});
export type ProofKeyValuePayload = z.infer<typeof proofKeyValuePayloadSchema>;

// P426: a point is [x, y]; x is a category label, epoch milliseconds or a number by x.type, y is
// null for a gap. Go normalises (time to epoch ms, line and area sorted by x).
export const proofChartPointSchema = /*#__PURE__*/ z.tuple([
  z.union([z.string(), z.number()]),
  z.number().nullable(),
]);

export const proofCartesianChartSchema = /*#__PURE__*/ z.object({
  type: z.enum(['bar', 'line', 'area', 'scatter']),
  x: z.object({ type: z.enum(['category', 'time', 'value']), name: z.string() }),
  y: z.object({ name: z.string(), unit: z.string() }),
  stacked: z.boolean(),
  horizontal: z.boolean(),
  series: z.array(z.object({ name: z.string(), points: z.array(proofChartPointSchema) })),
});
export type ProofCartesianChart = z.infer<typeof proofCartesianChartSchema>;

export const proofPieChartSchema = /*#__PURE__*/ z.object({
  type: z.literal('pie'),
  unit: z.string(),
  slices: z.array(z.object({ name: z.string(), value: z.number() })),
});
export type ProofPieChart = z.infer<typeof proofPieChartSchema>;

export const proofChartPayloadSchema = /*#__PURE__*/ z.union([
  proofCartesianChartSchema,
  proofPieChartSchema,
]);
export type ProofChartPayload = z.infer<typeof proofChartPayloadSchema>;

// Legend entries hidden, the clicked point and the zoom window (percent of the axis).
export const proofChartViewStateSchema = /*#__PURE__*/ z.object({
  hidden: z.array(z.string()),
  selected: z.object({ series: z.number().int(), index: z.number().int() }).nullable(),
  zoom: z.object({ start: z.number(), end: z.number() }).nullable(),
});
export type ProofChartViewState = z.infer<typeof proofChartViewStateSchema>;

// Mirrors views/shared/slick/selection.ts's Selection.
export const proofSelectionSchema = /*#__PURE__*/ z.discriminatedUnion('kind', [
  z.object({ kind: z.literal('cell'), row: z.number().int(), col: z.number().int() }),
  z.object({
    kind: z.literal('range'),
    anchorRow: z.number().int(),
    anchorCol: z.number().int(),
    row: z.number().int(),
    col: z.number().int(),
  }),
  z.object({ kind: z.literal('row'), rows: z.array(z.number().int()) }),
  z.object({ kind: z.literal('column'), cols: z.array(z.number().int()) }),
]);

// Where a tab was scrolled and what was highlighted: restored on every mount, matched by the
// screenshot, saved per step.
export const proofViewStateSchema = /*#__PURE__*/ z.object({
  queryScrollTop: z.number(),
  resultScrollTop: z.number(),
  resultScrollLeft: z.number(),
  selection: proofSelectionSchema.nullable(),
  selectedRow: z.number().int().nullable(),
  expanded: z.array(z.string()),
  columnWidths: z.record(z.string(), z.number()),
  // Absent on every step stored before P426 and on non-chart steps.
  chart: proofChartViewStateSchema.optional(),
});
export type ProofViewState = z.infer<typeof proofViewStateSchema>;

export const proofSummarySchema = /*#__PURE__*/ z.object({
  id: z.string(),
  name: z.string(),
  closedAt: z.number(),
  steps: z.number().int(),
  pending: z.boolean(),
  seen: z.boolean(),
});
export type ProofSummary = z.infer<typeof proofSummarySchema>;

export const proofStepMetaSchema = /*#__PURE__*/ z.object({
  ordinal: z.number().int(),
  description: z.string(),
  language: proofLanguageSchema,
  source: z.string(),
  view: proofViewSchema,
  bytes: z.number().int(),
  diff: proofDiffSchema,
  originOrdinal: z.number().int().nullable(),
});
export type ProofStepMeta = z.infer<typeof proofStepMetaSchema>;

export const proofRevisionSchema = /*#__PURE__*/ z.object({
  rev: z.number().int(),
  reason: z.string(),
  createdAt: z.number(),
  steps: z.array(proofStepMetaSchema),
});
export type ProofRevision = z.infer<typeof proofRevisionSchema>;

export const proofDetailSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  name: z.string(),
  summary: z.string(),
  closedAt: z.number(),
  current: proofRevisionSchema,
  pending: proofRevisionSchema.nullable(),
  removed: z.array(proofStepMetaSchema),
});
export type ProofDetail = z.infer<typeof proofDetailSchema>;

export const proofStepDetailSchema = /*#__PURE__*/ proofStepMetaSchema.extend({
  query: z.string(),
  data: z.unknown(),
});
export type ProofStepDetail = z.infer<typeof proofStepDetailSchema>;

export const proofViewStateEntrySchema = /*#__PURE__*/ z.object({
  ordinal: z.number().int(),
  state: proofViewStateSchema,
});
export type ProofViewStateEntry = z.infer<typeof proofViewStateEntrySchema>;

export const proofOpenResultSchema = /*#__PURE__*/ z.object({
  key: z.string(),
  opened: z.boolean(),
});
export const proofWindowTargetSchema = /*#__PURE__*/ z.object({ sessionId: z.string().nullable() });
export const proofSavePngResultSchema = /*#__PURE__*/ z.object({
  saved: z.boolean(),
  path: z.string().nullable(),
});
export const proofShotFolderResultSchema = /*#__PURE__*/ z.object({
  canceled: z.boolean(),
  id: z.string(),
  path: z.string(),
});
export const proofSavePngToResultSchema = /*#__PURE__*/ z.object({ path: z.string() });
export type ProofShotFolderResult = z.infer<typeof proofShotFolderResultSchema>;
export type ProofSavePngToResult = z.infer<typeof proofSavePngToResultSchema>;
export type ProofOpenResult = z.infer<typeof proofOpenResultSchema>;
export type ProofWindowTarget = z.infer<typeof proofWindowTargetSchema>;
export type ProofSavePngResult = z.infer<typeof proofSavePngResultSchema>;

// kira:proof:changed and kira:proof:select payloads.
export const proofChangedSchema = /*#__PURE__*/ z.object({ sessionId: z.string() });
export const proofSelectSchema = /*#__PURE__*/ z.object({ sessionId: z.string() });
export type ProofChanged = z.infer<typeof proofChangedSchema>;
export type ProofSelect = z.infer<typeof proofSelectSchema>;

// P520: payload of a proof.review approval item (internal/proof putReview).
export const proofReviewPayloadSchema = /*#__PURE__*/ z.object({
  sessionId: z.string(),
  name: z.string(),
  revision: z.number(),
  reason: z.string(),
  steps: z.array(z.object({ ordinal: z.number(), description: z.string(), diff: z.string() })),
});
export type ProofReviewPayload = z.infer<typeof proofReviewPayloadSchema>;
