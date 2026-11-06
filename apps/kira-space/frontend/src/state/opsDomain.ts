import { opLogRecordSchema } from '@shared/domain/ops';
import { z } from 'zod';

// P132 Part 2: Kira Space's git op record — the shared op-log base plus the repo, origin and
// cancellability the Go ring (internal/oplog) adds. Type only: the bridge trusts the wire shape,
// like every sibling method.
const opStepSchema = /*#__PURE__*/ z.object({
  argv: z.string(),
  readOnly: z.boolean(),
  exitCode: z.number().nullable(),
  durationMs: z.number().nullable(),
  status: z.enum(['running', 'ok', 'error', 'cancelled']),
  stderr: z.string().nullable(),
});
export type SpaceOpStep = z.infer<typeof opStepSchema>;

const spaceOpRecordSchema = /*#__PURE__*/ opLogRecordSchema.extend({
  category: z.string(),
  module: z.string(),
  trigger: z.string(),
  subject: z.string(),
  repoRoot: z.string(),
  repoName: z.string(),
  source: z.string(),
  taskId: z.string(),
  parentId: z.string(),
  background: z.boolean(),
  repeat: z.number(),
  steps: z.array(opStepSchema),
  stepsDropped: z.number(),
  cancellable: z.boolean(),
  // Database and API ops only.
  connectionId: z.string().nullable().optional(),
  tabId: z.string().nullable().optional(),
  rows: z.number().nullable().optional(),
  commandTruncated: z.boolean().optional(),
  path: z.string().nullable().optional(),
  delayMs: z.number().optional(),
});
export type SpaceOpRecord = z.infer<typeof spaceOpRecordSchema>;
