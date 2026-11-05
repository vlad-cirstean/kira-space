import { z } from 'zod';

// P520: payload of a branch.request approval item (internal/ade branchPayload). Item ids are
// c<call>i<k>: concurrent calls of one task merge into one item.
const branchRequestItemSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  codeRepoId: z.string(),
  repo: z.string(),
  kind: z.enum(['branch', 'worktree', 'rename']),
  branch: z.string(),
  base: z.string(),
  from: z.string(),
  upstream: z.string(),
  stacked: z.array(z.string()),
  source: z.string(),
  stepName: z.string(),
  session: z.string(),
});
export type BranchRequestItem = z.infer<typeof branchRequestItemSchema>;

export const branchRequestPayloadSchema = /*#__PURE__*/ z.object({
  taskId: z.string(),
  taskTitle: z.string(),
  items: z.array(branchRequestItemSchema),
});
