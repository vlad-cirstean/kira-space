// P512: the trackable concepts per item kind, in display order. Mirrors updates.Catalog in
// apps/kira-space/internal/updates/catalog.go; settings-updates.spec.ts proves the two equal.
export const TRACKED_KINDS = [
  {
    kind: 'changeRequest',
    concepts: [
      { id: 'opened', default: true },
      { id: 'commentAdded', default: true },
      { id: 'merged', default: true },
      { id: 'closed', default: true },
      { id: 'reopened', default: true },
      { id: 'readyForReview', default: true },
      { id: 'checksFailed', default: true },
      { id: 'checksPassed', default: false },
      { id: 'approved', default: true },
      { id: 'changesRequested', default: true },
      { id: 'newCommits', default: false },
      { id: 'titleChanged', default: false },
      { id: 'assigneeChanged', default: false },
    ],
  },
  {
    kind: 'issue',
    concepts: [
      { id: 'statusChanged', default: true },
      { id: 'commentAdded', default: true },
      { id: 'assigneeChanged', default: false },
      { id: 'priorityChanged', default: false },
      { id: 'titleChanged', default: false },
      { id: 'linkedChangeRequests', default: false },
    ],
  },
  {
    kind: 'branch',
    concepts: [
      { id: 'remoteCommits', default: true },
      { id: 'merged', default: true },
      { id: 'conflicts', default: true },
      { id: 'deploymentChanged', default: true },
      { id: 'baseMoved', default: false },
      { id: 'localCommits', default: false },
    ],
  },
] as const;

export type TrackedKind = (typeof TRACKED_KINDS)[number]['kind'];

export function trackingDefaults(kind: TrackedKind): Record<string, boolean> {
  const entry = TRACKED_KINDS.find((k) => k.kind === kind);
  return Object.fromEntries((entry?.concepts ?? []).map((c) => [c.id, c.default]));
}
