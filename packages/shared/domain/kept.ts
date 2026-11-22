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
  // statement and param belong to a SQL run_query: the placeholder in the statement, and the params entry.
  field: z.enum([
    'url',
    'header',
    'query',
    'form',
    'body',
    'contentType',
    'script',
    'statement',
    'param',
  ]),
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
  // A value the user has yet to type (a needed input): no id, no value; the sites still mark where it goes.
  pending: z.boolean().default(false),
  sites: z
    .array(keptSiteSchema)
    .nullish()
    .transform((v) => v ?? []),
});
export type KeptUse = z.infer<typeof keptUseSchema>;

export const keptEntrySchema = /*#__PURE__*/ z.object({
  ...keptMetaShape,
  scopeLabel: z.string(),
  // scheme://host[:port] a user-supplied value goes to; empty for a script's value.
  origin: z.string().default(''),
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

// A value a script will save, or a saved value it deletes (bridge DbMcpPendingKeep): a preview, empty for a
// secret, never stored yet. A deleted row describes the value it removes.
export const keptStagedSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  type: z.string(),
  items: z.number(),
  bytes: z.number(),
  secret: z.boolean(),
  preview: z.string(),
  previewTruncated: z.boolean(),
  deleted: z.boolean().default(false),
});
export type KeptStaged = z.infer<typeof keptStagedSchema>;
