import { z } from 'zod';

// internal/keptvars and bridge/keptvars.go: a kept variable as the approval card and the Settings
// view show it. A secret has no preview. The agent never receives any of this.
const keptSourceSchema = /*#__PURE__*/ z.object({
  kind: z.string(),
  mcp: z.string(),
  tool: z.string(),
  request: z.string(),
  client: z.string(),
  taskId: z.string().optional(),
  sessionId: z.string().optional(),
});

// A place a request or script reads the variable: offsets are UTF-16 units of the field's text. For
// `script`, `part` is the stage (`prepare` or `transform`) and the offsets index its source.
export const keptSiteSchema = /*#__PURE__*/ z.object({
  field: z.enum(['url', 'header', 'query', 'form', 'body', 'contentType', 'script']),
  index: z.number(),
  part: z.string(),
  from: z.number(),
  to: z.number(),
});
export type KeptSite = z.infer<typeof keptSiteSchema>;

const keptMetaShape = {
  id: z.string(),
  name: z.string(),
  type: z.string(),
  items: z.number(),
  bytes: z.number(),
  secret: z.boolean(),
  source: keptSourceSchema,
  createdAt: z.string(),
  expiresAt: z.string(),
  preview: z.string(),
  previewTruncated: z.boolean(),
};

// A variable a pending request uses (agentapi.KeptUse).
export const keptUseSchema = /*#__PURE__*/ z.object({
  ...keptMetaShape,
  sites: z
    .array(keptSiteSchema)
    .nullish()
    .transform((v) => v ?? []),
});
export type KeptUse = z.infer<typeof keptUseSchema>;

export const keptEntrySchema = /*#__PURE__*/ z.object({
  ...keptMetaShape,
  scopeLabel: z.string(),
});
export type KeptEntry = z.infer<typeof keptEntrySchema>;

export const keptListSchema = /*#__PURE__*/ z.object({
  entries: z
    .array(keptEntrySchema)
    .nullish()
    .transform((v) => v ?? []),
  retentionMinutes: z.number(),
});
export type KeptList = z.infer<typeof keptListSchema>;

export const keptValueSchema = /*#__PURE__*/ z.object({ json: z.string(), type: z.string() });
export type KeptValue = z.infer<typeof keptValueSchema>;

// A value a held result will keep (bridge DbMcpPendingKeep): a preview, empty for a secret, never stored yet.
export const keptStagedSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  type: z.string(),
  items: z.number(),
  bytes: z.number(),
  secret: z.boolean(),
  preview: z.string(),
  previewTruncated: z.boolean(),
});
export type KeptStaged = z.infer<typeof keptStagedSchema>;
