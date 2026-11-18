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

/** Contract ops with the masked id and start time (hydrated to '') given real values; an empty start time cannot format. */
export function unmaskedOps(ops: Record<string, unknown>[]): Record<string, unknown>[] {
  return ops.map((op, i) => ({
    ...op,
    id: op.id || `op-contract-${i}`,
    startedAt: op.startedAt || '2026-01-01T00:00:00.000Z',
  }));
}
