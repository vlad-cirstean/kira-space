// How each product shot is captured. Zero imports: the site (Astro) and the shot specs
// (apps/kira-space/tsconfig.tests.json) both load this file. Site copy lives in shots.ts.

export type ShotCapture =
  | { readonly kind: 'window'; readonly detail: { readonly x: number; readonly y: number } }
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
  'studio-data-grid': { kind: 'window', detail: { x: 265, y: 130 } },
  'studio-sql-console': { kind: 'window', detail: { x: 265, y: 150 } },
  'studio-api': { kind: 'window', detail: { x: 270, y: 310 } },
  'space-git-graph': { kind: 'window', detail: { x: 265, y: 130 } },
  'space-agents-board': { kind: 'window', detail: { x: 290, y: 180 } },
  'space-memory': { kind: 'window', detail: { x: 0, y: 130 } },
  'studio-grid-row-menu': { kind: 'floating', selector: '[data-testid="context-menu"]' },
  'space-commit-detail': {
    kind: 'region',
    selector: '[data-testid="detail-region"]',
    top: '[data-testid="commit-meta"]',
    bottom: '[data-testid="file-tree-row"]',
    padding: 0,
  },
  // Build with Kira (P551)
  'build-start-board': { kind: 'window', detail: { x: 120, y: 280 } },
  'build-start-capture': { kind: 'floating', selector: '[data-testid="ade-add-popover"]' },
  'build-start-workflow': { kind: 'window', detail: { x: 320, y: 150 } },
  'build-start-step': { kind: 'region', selector: '[data-testid="ade-wf-inspector"]' },
  'build-start-launch': { kind: 'floating', selector: '[data-testid="context-menu"]' },
  'build-db-approval': { kind: 'window', detail: { x: 300, y: 120 } },
  'build-db-statement': { kind: 'region', selector: '[data-testid^="approval-item-db.query"]' },
  'build-db-exemption': { kind: 'region', selector: '[data-testid^="approval-item-db.exemption"]' },
  'build-db-audit': { kind: 'region', selector: '[data-testid="ade-details-tab"]', padding: 0 },
  'build-db-settings': { kind: 'region', selector: '[data-testid="settings-dialog"]', padding: 0 },
  'build-branches-panel': { kind: 'window', detail: { x: 600, y: 120 } },
  'build-branches-add': { kind: 'floating', selector: '[data-testid="ade-add-popover"]' },
  'build-handover-dialog': { kind: 'floating', selector: '[data-testid="ade-stage-dialog"]' },
  'build-handover-notes': { kind: 'region', selector: '[data-testid="ade-task-chain"]' },
  'build-artifacts-list': { kind: 'region', selector: '[data-testid="ade-details-links"]' },
  'build-artifacts-done': { kind: 'floating', selector: '[data-testid="ade-stage-dialog"]' },
  'build-review-window': { kind: 'window', detail: { x: 290, y: 100 } },
  'build-review-thread': { kind: 'region', selector: '[data-testid="repo-diff-editor"]' },
  'build-review-agent': { kind: 'region', selector: '[data-testid="ade-review-right"]' },
  'build-review-forward': { kind: 'region', selector: '[data-testid="ade-review-forward-bar"]' },
  'build-review-actions': { kind: 'floating', selector: '[role="menu"]' },
  'build-proof-window': { kind: 'window', detail: { x: 0, y: 0 } },
  'build-proof-chart': { kind: 'window', detail: { x: 0, y: 0 } },
  'build-proof-diagram': { kind: 'region', selector: '[data-testid="proof-step"]' },
  'build-proof-compare': { kind: 'window', detail: { x: 0, y: 0 } },
  'build-proof-chain': { kind: 'region', selector: '[data-testid="ade-task-chain"]' },
} as const satisfies Record<string, ShotCapture>;

export type ShotId = keyof typeof SHOT_CAPTURE;

/** Files under screenshots/ that one shot writes: the single source for site, fill script and cache. */
export function shotFiles(id: ShotId): { main: string; phone?: string } {
  const capture: ShotCapture = SHOT_CAPTURE[id];
  return capture.kind === 'window'
    ? { main: `framed/${id}.webp`, phone: `${id}-detail.webp` }
    : { main: `crop/${id}.webp` };
}
