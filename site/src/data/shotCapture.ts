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
} as const satisfies Record<string, ShotCapture>;

export type ShotId = keyof typeof SHOT_CAPTURE;

/** Files under screenshots/ that one shot writes: the single source for site, fill script and cache. */
export function shotFiles(id: ShotId): { main: string; phone?: string } {
  const capture: ShotCapture = SHOT_CAPTURE[id];
  return capture.kind === 'window'
    ? { main: `framed/${id}.webp`, phone: `${id}-detail.webp` }
    : { main: `crop/${id}.webp` };
}
