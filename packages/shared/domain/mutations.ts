import { z } from 'zod';
import type { NodePath } from './tree';

const textValueSchema = z.string().nullable();
const rowValuesSchema = /*#__PURE__*/ z.record(z.string(), textValueSchema);

// The wire form's `ops` array is exactly this shape (data-ops.ts's MutateRequestWire/
// PreviewRequestWire carry `path` separately, as an encoded string — D13).
export const mutationRowOpSchema = /*#__PURE__*/ z.discriminatedUnion('kind', [
  /*#__PURE__*/ z.object({
    kind: z.literal('update'),
    key: rowValuesSchema,
    changes: rowValuesSchema,
    // Loaded values of guarded columns: a relational adapter fails the op with E_CONFLICT when the
    // row no longer holds them.
    expect: rowValuesSchema.optional(),
  }),
  /*#__PURE__*/ z.object({ kind: z.literal('insert'), values: rowValuesSchema }),
  /*#__PURE__*/ z.object({
    kind: z.literal('delete'),
    key: rowValuesSchema,
    expect: rowValuesSchema.optional(),
  }),
]);
export type MutationRowOp = z.infer<typeof mutationRowOpSchema>;

// Adapter-side only (D1): always one table. Constructed by engine/data.ts from a decoded
// NodePath plus the wire's own `ops` — never parsed as a whole from an external boundary.
export interface MutationPlan {
  path: NodePath;
  ops: MutationRowOp[];
}

export interface MutationResult {
  affectedRows: number;
}
