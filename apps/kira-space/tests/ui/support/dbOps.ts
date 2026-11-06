/** A database op as the merged op log carries it: the Space record fields a hand-written fixture leaves out. */
export function dbOp(record: Record<string, unknown>): Record<string, unknown> {
  return {
    category: 'db',
    module: 'database',
    trigger: record.source ?? 'user',
    subject: '',
    repoRoot: '',
    repoName: '',
    taskId: '',
    parentId: '',
    background: false,
    repeat: 1,
    steps: [],
    stepsDropped: 0,
    cancellable: record.status === 'running',
    ...record,
  };
}
