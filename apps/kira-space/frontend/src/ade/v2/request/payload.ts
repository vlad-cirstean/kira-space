import { z } from 'zod';

// P520: payload of a branch.request approval item (internal/ade branchPayload). Item ids are
// c<call>i<k>: concurrent calls of one task merge into one item.
const branchRequestItemSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  codeRepoId: z.string(),
  repo: z.string(),
  kind: z.enum(['rename', 'switch', 'adopt']),
  branch: z.string(),
  base: z.string(),
  from: z.string(),
  upstream: z.string(),
  stacked: z.array(z.string()),
  source: z.string(),
  stepName: z.string(),
  session: z.string(),
  // 'adopt': where the branch is checked out now, and the uncommitted changes that stay there.
  holder: z
    .object({
      kind: z.enum(['task', 'main', 'other', 'none']),
      taskId: z.string(),
      branchId: z.string(),
      worktree: z.string(),
    })
    .optional(),
  dirty: z.number().optional(),
});
export type BranchRequestItem = z.infer<typeof branchRequestItemSchema>;

export const branchRequestPayloadSchema = /*#__PURE__*/ z.object({
  taskId: z.string(),
  taskTitle: z.string(),
  items: z.array(branchRequestItemSchema),
});

// payload of a task.create approval item (internal/ade taskCreatePayload): an orchestrator's request
// for one child task. Approve or deny only; the brief becomes the child's first handover.
export const taskCreatePayloadSchema = /*#__PURE__*/ z.object({
  parentTaskId: z.string(),
  parentTitle: z.string(),
  title: z.string(),
  workflow: z.object({ id: z.string(), name: z.string() }),
  repos: z.array(z.object({ codeRepoId: z.string(), name: z.string(), base: z.string() })),
  brief: z.string(),
});

// payload of a details.edit approval item (internal/ade detailsPayload): the task-folder files of one task
// that wait for the user. Options answer {approved: [paths]}; a path left out is restored.
const detailsEditFileSchema = /*#__PURE__*/ z.object({
  path: z.string(),
  kind: z.string(),
  cause: z.string(),
  text: z.string(),
  reason: z.string(),
  collision: z.string().optional(),
});

export const detailsEditPayloadSchema = /*#__PURE__*/ z.object({
  taskId: z.string(),
  taskTitle: z.string(),
  reason: z.string(),
  items: z.array(detailsEditFileSchema),
});
