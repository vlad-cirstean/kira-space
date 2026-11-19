const APP = 'apps/kira-space/frontend/';
const WORKBENCH_AREAS = new Set([
  'memory',
  'automations',
  'agent',
  'approvals',
  'settings',
  'terminal',
  'editor',
  'tabs',
  'prompt',
  'restore',
]);

function segment(parts: string[], i: number): string {
  return parts[i] ?? '';
}

function studioModule(parts: string[]): string {
  const area = segment(parts, 1);
  if (area === 'views') return `studio/views/${segment(parts, 2)}`;
  if (area.includes('.')) return 'studio';
  return `studio/${area}`;
}

function adeModule(parts: string[]): string {
  const area = segment(parts, 1) === 'v2' ? segment(parts, 2) : segment(parts, 1);
  return area.includes('.') ? 'ade' : `ade/${area}`;
}

function appSrcModule(parts: string[]): string {
  const top = segment(parts, 0);
  if (top === 'studio') return studioModule(parts);
  if (top === 'ade') return adeModule(parts);
  if (top === 'workbench') return 'app-workbench';
  return top.includes('.') ? 'app' : top;
}

/** Module of a repo-relative path: the first path segment that names an area. */
export function moduleOf(rel: string): string {
  if (rel.startsWith(`${APP}mobile/`)) return 'mobile';
  if (rel.startsWith(`${APP}src/`)) return appSrcModule(rel.slice(`${APP}src/`.length).split('/'));
  if (rel.startsWith(APP)) return 'app';
  const m = /^packages\/([^/]+)\/src\/(.*)$/.exec(rel);
  if (!m) return 'other';
  const [, pkg, rest] = m;
  if (pkg !== 'workbench') return pkg;
  const area = rest.split('/')[0];
  return WORKBENCH_AREAS.has(area) ? `workbench/${area}` : 'workbench';
}
