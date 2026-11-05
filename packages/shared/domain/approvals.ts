import { z } from 'zod';

// P520: one pending agent approval (internal/approvals.Item). `kind` is the subject key the client
// registry maps to a body renderer; `payload` is per kind and validated by that renderer's schema.
export const approvalOperationToneSchema = /*#__PURE__*/ z.enum(['neutral', 'caution', 'danger']);
export type ApprovalOperationTone = z.infer<typeof approvalOperationToneSchema>;

export const approvalItemSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  kind: z.string(),
  createdAt: z.number(),
  // 0 = no expiry.
  expiresAt: z.number(),
  origin: z.object({
    client: z.string(),
    cwd: z.string(),
    task: z.string(),
    step: z.string(),
    source: z.string(),
  }),
  target: z.object({
    name: z.string(),
    kind: z.string(),
    color: z.string(),
    readOnly: z.boolean(),
    location: z.string(),
  }),
  operation: z.object({ class: z.string(), tone: approvalOperationToneSchema }),
  reason: z.string(),
  payload: z.unknown(),
});
export type ApprovalItem = z.infer<typeof approvalItemSchema>;

export const approvalResultSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  code: z.string(),
  error: z.string(),
});
export type ApprovalResult = z.infer<typeof approvalResultSchema>;

export interface ApprovalAnswerArgs {
  ids: string[];
  approve: boolean;
  /** Per-id choices a kind renderer collected (branch.request: itemIds, remoteItemIds). */
  options?: Record<string, unknown>;
}
