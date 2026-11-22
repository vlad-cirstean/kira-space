// How each product shot is captured. Zero imports: the site (Astro) and the shot specs
// (apps/kira-space/tsconfig.tests.json) both load this file. Site copy lives in shots.ts.

export type ShotCapture =
  | { readonly kind: 'window' }
  /** The phone app at iPhone 15 size, in rounded corners with a shadow. */
  | { readonly kind: 'phone' }
  | { readonly kind: 'floating'; readonly selector: string; readonly padding?: number }
  | {
      readonly kind: 'region';
      readonly selector: string;
      readonly padding?: number;
      /** Vertical trim: from the top of the first `top` match to the bottom of the last `bottom` match. */
      readonly top?: string;
      readonly bottom?: string;
    };

export const SHOT_CAPTURE = {
  'studio-data-grid': { kind: 'window' },
  'studio-sql-console': { kind: 'window' },
  'studio-connections': { kind: 'window' },
  'studio-docker': { kind: 'window' },
  'studio-api': { kind: 'window' },
  'space-git-graph': { kind: 'window' },
  'space-agents-board': { kind: 'window' },
  'space-workspace': { kind: 'window' },
  'space-runs': { kind: 'window' },
  'studio-grid-row-menu': { kind: 'floating', selector: '[data-testid="context-menu"]' },
  'space-commit-detail': {
    kind: 'region',
    selector: '[data-testid="detail-region"]',
    top: '[data-testid="commit-meta"]',
    bottom: '[data-testid="file-tree-row"]',
    padding: 0,
  },
  // Build with Kira (P551)
  'build-start-board': { kind: 'window' },
  'build-start-capture': { kind: 'floating', selector: '[data-testid="ade-add-popover"]' },
  'build-start-workflow': { kind: 'window' },
  'build-start-step': {
    kind: 'region',
    selector: '[data-testid="ade-wf-inspector"]',
    // Ends at the status row: the sandbox fields below sit under the panel's scroll edge.
    bottom: 'text=Edit statuses',
    padding: 0,
  },
  'build-start-launch': { kind: 'floating', selector: '[data-testid="context-menu"]' },
  'build-db-approval': { kind: 'window' },
  // Approval prompts crop the whole dialog (header, card, actions) at its full content height.
  'build-db-statement': { kind: 'floating', selector: '[data-testid="approvals-dialog"]' },
  'build-db-exemption': { kind: 'floating', selector: '[data-testid="approvals-dialog"]' },
  'build-db-audit': { kind: 'region', selector: '[data-testid="ade-details-tab"]', padding: 0 },
  'studio-mcp-settings': {
    kind: 'region',
    selector: '[data-testid="settings-dialog"]',
    padding: 0,
  },
  'build-branches-panel': { kind: 'window' },
  'build-branches-add': { kind: 'floating', selector: '[data-testid="ade-add-popover"]' },
  'build-handover-dialog': { kind: 'floating', selector: '[data-testid="ade-stage-dialog"]' },
  'build-handover-notes': { kind: 'region', selector: '[data-testid="ade-task-chain"]' },
  'build-artifacts-list': { kind: 'region', selector: '[data-testid="ade-links"]' },
  'build-artifacts-done': { kind: 'floating', selector: '[data-testid="ade-stage-dialog"]' },
  'build-review-window': { kind: 'window' },
  'build-review-thread': {
    kind: 'region',
    selector: '[data-testid="repo-diff-editor"]',
    padding: 0,
  },
  'build-review-agent': {
    kind: 'region',
    selector: '[data-testid="ade-review-right"]',
    padding: 0,
  },
  'build-review-forward': {
    kind: 'region',
    selector: '[data-testid="ade-review-forward-bar"]',
    padding: 0,
  },
  'build-review-actions': { kind: 'floating', selector: '[role="menu"]' },
  'build-proof-window': { kind: 'window' },
  'build-proof-chart': { kind: 'window' },
  'build-proof-diagram': {
    kind: 'region',
    selector: '[data-testid="proof-step"]',
    bottom: '[data-testid="proof-diagram"] svg',
  },
  'build-proof-compare': { kind: 'window' },
  'build-proof-chain': { kind: 'region', selector: '[data-testid="ade-task-chain"]' },
  // Phone app (P734)
  'phone-plan': { kind: 'phone' },
  'phone-needs': { kind: 'phone' },
  'phone-terminal': { kind: 'phone' },
  'phone-activity': { kind: 'phone' },
  // Agent API requests (P734)
  'build-api-request': { kind: 'floating', selector: '[data-testid="approvals-dialog"]' },
  'build-api-review': { kind: 'floating', selector: '[data-testid="approvals-dialog"]' },
  'studio-api-values': { kind: 'floating', selector: '[data-testid="approvals-dialog"]' },
  'studio-saved-variables': {
    kind: 'region',
    selector: '[data-testid="settings-dialog"]',
    padding: 0,
  },
  // Kubernetes, MongoDB and the Files window (P734)
  'studio-kube-read': { kind: 'floating', selector: '[data-testid="approvals-dialog"]' },
  'studio-kube-review': { kind: 'floating', selector: '[data-testid="approvals-dialog"]' },
  'studio-api-variables': { kind: 'window' },
  'studio-kube-pods': { kind: 'window' },
  'studio-kube-logs': {
    kind: 'region',
    selector: '[data-testid="kube-detail"][data-active="true"]',
    bottom: '[data-testid="kube-logs-view-line"]',
    padding: 4,
  },
  'studio-mongo-documents': { kind: 'window' },
  'space-files-window': { kind: 'window' },
} as const satisfies Record<string, ShotCapture>;

export type ShotId = keyof typeof SHOT_CAPTURE;

/** Files under screenshots/ that one shot writes: the single source for site, fill script and cache. */
export function shotFiles(id: ShotId): { main: string } {
  const capture: ShotCapture = SHOT_CAPTURE[id];
  if (capture.kind === 'window') return { main: `framed/${id}.webp` };
  return { main: capture.kind === 'phone' ? `phone/${id}.webp` : `crop/${id}.webp` };
}
