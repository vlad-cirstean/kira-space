// How each product shot is captured. Zero imports: the site (Astro) and the shot specs
// (apps/kira-space/tsconfig.tests.json) both load this file. Site copy lives in shots.ts.

export type ShotCapture =
  | { readonly kind: 'window' }
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
  'studio-api': { kind: 'window' },
  'space-git-graph': { kind: 'window' },
  'space-agents-board': { kind: 'window' },
  'space-memory': { kind: 'window' },
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
  'build-start-step': { kind: 'region', selector: '[data-testid="ade-wf-inspector"]' },
  'build-start-launch': { kind: 'floating', selector: '[data-testid="context-menu"]' },
  'build-db-approval': { kind: 'window' },
  'build-db-statement': { kind: 'region', selector: '[data-testid^="approval-item-db.query"]' },
  'build-db-exemption': { kind: 'region', selector: '[data-testid^="approval-item-db.exemption"]' },
  'build-db-audit': { kind: 'region', selector: '[data-testid="ade-details-tab"]', padding: 0 },
  'build-db-settings': { kind: 'region', selector: '[data-testid="settings-dialog"]', padding: 0 },
  'build-branches-panel': { kind: 'window' },
  'build-branches-add': { kind: 'floating', selector: '[data-testid="ade-add-popover"]' },
  'build-handover-dialog': { kind: 'floating', selector: '[data-testid="ade-stage-dialog"]' },
  'build-handover-notes': { kind: 'region', selector: '[data-testid="ade-task-chain"]' },
  'build-artifacts-list': { kind: 'region', selector: '[data-testid="ade-details-links"]' },
  'build-artifacts-done': { kind: 'floating', selector: '[data-testid="ade-stage-dialog"]' },
  'build-review-window': { kind: 'window' },
  'build-review-thread': { kind: 'region', selector: '[data-testid="repo-diff-editor"]' },
  'build-review-agent': { kind: 'region', selector: '[data-testid="ade-review-right"]' },
  'build-review-forward': { kind: 'region', selector: '[data-testid="ade-review-forward-bar"]' },
  'build-review-actions': { kind: 'floating', selector: '[role="menu"]' },
  'build-proof-window': { kind: 'window' },
  'build-proof-chart': { kind: 'window' },
  'build-proof-diagram': { kind: 'region', selector: '[data-testid="proof-step"]' },
  'build-proof-compare': { kind: 'window' },
  'build-proof-chain': { kind: 'region', selector: '[data-testid="ade-task-chain"]' },
} as const satisfies Record<string, ShotCapture>;

export type ShotId = keyof typeof SHOT_CAPTURE;

/** Files under screenshots/ that one shot writes: the single source for site, fill script and cache. */
export function shotFiles(id: ShotId): { main: string } {
  const capture: ShotCapture = SHOT_CAPTURE[id];
  return { main: capture.kind === 'window' ? `framed/${id}.webp` : `crop/${id}.webp` };
}
