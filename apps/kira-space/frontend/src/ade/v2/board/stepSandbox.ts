import type { SandboxAccess, StepKind, StepSandbox } from '../wire';

/** The policy of a step without a sandbox block: scripts run unconfined, the rest write their worktree. */
export function defaultSandbox(kind: StepKind): StepSandbox {
  return { access: kind === 'script' ? 'off' : 'write', read: [], allow: [], network: true };
}

export const ACCESS_MODES: readonly SandboxAccess[] = ['write', 'read', 'off'];

export const TOOLCHAINS = [
  { id: 'go', paths: ['~/go', '~/Library/Caches/go-build'] },
  { id: 'node', paths: ['~/.npm', '~/.nvm', '~/.bun'] },
  { id: 'rust', paths: ['~/.cargo', '~/.rustup'] },
  { id: 'python', paths: ['~/.pyenv', '~/Library/Caches/pip'] },
] as const;

/** One path per line; blank lines drop. */
export function parsePaths(text: string): string[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean);
}

export function formatPaths(paths: readonly string[]): string {
  return paths.join('\n');
}

/** Normalizes `.` and `..` segments, like Go's `path.Clean`. */
function cleanPath(p: string): string {
  const out: string[] = [];
  for (const seg of p.split('/')) {
    if (seg === '' || seg === '.') continue;
    if (seg === '..') {
      if (out.length && out[out.length - 1] !== '..') out.pop();
      else if (!p.startsWith('/')) out.push('..');
      continue;
    }
    out.push(seg);
  }
  const joined = out.join('/');
  return p.startsWith('/') ? `/${joined}` : joined || '.';
}

/** Mirrors the workflow parser: roots, home, ~/Library, ~/.claude and the keychain directory are refused, in `~` or absolute spelling. */
function isProtectedPath(p: string): boolean {
  const c = cleanPath(p);
  if (['/', '~', '~/Library', '~/.claude', '.'].includes(c)) return true;
  return (
    c.startsWith('..') || `${c}/`.startsWith('~/Library/Keychains/') || isProtectedAbsoluteHome(c)
  );
}

/** Absolute spelling of the protected paths: /Users or /home, a home folder, its Library and .claude, the keychain directory. */
function isProtectedAbsoluteHome(c: string): boolean {
  const seg = c.split('/').slice(1);
  if (!c.startsWith('/') || (seg[0] !== 'Users' && seg[0] !== 'home')) return false;
  if (seg.length <= 2) return true;
  if (seg.length === 3) return seg[2] === 'Library' || seg[2] === '.claude';
  return seg[2] === 'Library' && seg[3] === 'Keychains';
}

export function pathsValid(paths: readonly string[]): boolean {
  return paths.every(
    (p) => (p.startsWith('/') || p === '~' || p.startsWith('~/')) && !isProtectedPath(p),
  );
}

export function addPaths(paths: readonly string[], add: readonly string[]): string[] {
  return [...paths, ...add.filter((p) => !paths.includes(p))];
}
