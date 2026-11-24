/**
 * U182: catalog key ownership. A module's sources read its own namespace plus the shared ones; a key
 * borrowed from another module is shared vocabulary and belongs in `common.*` (or the owner's
 * namespace). Named pairs (exemptions.ts OWNERSHIP_PAIRS) cover a module that hosts another's UI.
 */

/** File path prefix to the module's own catalog namespace; the first match wins. Files outside this table are not checked. */
export const OWNERSHIP_MODULES: readonly (readonly [prefix: string, namespace: string])[] = [
  ['apps/kira-space/frontend/src/studio/', 'studio'],
  ['apps/kira-space/frontend/src/ade/', 'space'],
  ['apps/kira-space/frontend/src/repo/', 'space'],
  ['apps/kira-space/frontend/src/views/', 'space'],
  ['apps/kira-space/frontend/src/capture/', 'space'],
  ['apps/kira-space/frontend/src/configsync/', 'space'],
  ['apps/kira-space/frontend/src/openedfiles/', 'space'],
  ['apps/kira-space/frontend/mobile/', 'mobile'],
  ['packages/docker-ui/src/', 'docker'],
  ['packages/git-ui/src/', 'git'],
  ['packages/workbench/src/automations/', 'automations'],
  ['packages/workbench/src/llmPrompts/', 'llmPrompts'],
  ['packages/workbench/src/', 'workbench'],
  ['apps/kira-space/frontend/src/workbench/', 'shell'],
  ['apps/kira-space/frontend/src/state/', 'shell'],
  ['apps/kira-space/frontend/src/shortcuts/', 'shell'],
  ['apps/kira-space/frontend/src/bridge/', 'shell'],
  ['apps/kira-space/frontend/src/App.vue', 'shell'],
  ['apps/kira-space/frontend/src/main.ts', 'shell'],
];

/**
 * Studio's Kubernetes view keeps its own top-level namespace; all of Studio's sources read both. The
 * shell is the Space app's frame: it reads `space` and has no namespace of its own.
 */
const ALSO_OWNS: Readonly<Record<string, readonly string[]>> = {
  studio: ['kubernetes'],
  shell: ['space'],
};

/** Namespaces every module may read. */
const SHARED_NAMESPACES: readonly string[] = ['common', 'format', 'shared', 'theme', 'backend'];

export interface OwnershipPair {
  /** Own namespace of the borrowing module. */
  module: string;
  /** Key prefix it may read, ending in a dot. */
  prefix: string;
  reason: string;
}

export interface OwnershipHit {
  file: string;
  line: number;
  key: string;
  module: string;
}

const KEY_LITERAL = /["'`]([a-z][a-zA-Z0-9]*(?:\.[a-zA-Z0-9]+){2,5})["'`]/g;

export function moduleOf(file: string): string | null {
  for (const [prefix, namespace] of OWNERSHIP_MODULES)
    if (file.startsWith(prefix)) return namespace;
  return null;
}

/** Catalog-key literals in a file that its module does not own and no shared namespace covers. */
export function scanOwnership(
  file: string,
  source: string,
  catalogKeys: ReadonlySet<string>,
): OwnershipHit[] {
  const module = moduleOf(file);
  if (module === null) return [];
  const allowed = [module, ...(ALSO_OWNS[module] ?? []), ...SHARED_NAMESPACES];
  const hits: OwnershipHit[] = [];
  for (const m of source.matchAll(KEY_LITERAL)) {
    const key = m[1] as string;
    if (!catalogKeys.has(key) || allowed.includes(key.slice(0, key.indexOf('.')))) continue;
    const line = source.slice(0, m.index).split('\n').length;
    hits.push({ file, line, key, module });
  }
  return hits;
}

export interface OwnershipResult {
  remaining: OwnershipHit[];
  errors: string[];
}

/** Drops hits a named pair covers; reports every pair that covers nothing. */
export function applyPairs(
  hits: readonly OwnershipHit[],
  pairs: readonly OwnershipPair[],
): OwnershipResult {
  const errors = pairs
    .filter((p) => !hits.some((h) => h.module === p.module && h.key.startsWith(p.prefix)))
    .map((p) => `stale ownership pair ${p.module} to ${p.prefix}, no U182 hit left (remove it)`);
  const remaining = hits.filter(
    (h) => !pairs.some((p) => p.module === h.module && h.key.startsWith(p.prefix)),
  );
  return { remaining, errors };
}
