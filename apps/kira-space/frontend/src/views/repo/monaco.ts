// The sole contact point with `monaco-editor` for the repo workspace's own
// URI/model-cache machinery. The engine-generic bootstrap (`loadMonaco`,
// `MonacoModule`, the theme definition) moved into `editor/monaco.ts`, shared by every editor
// surface in Kira Studio — re-exported here unchanged so this file's own consumers
// (RepoFileView.vue, RepoDiffView.vue) need no edit.
// Re-exported from `@workbench/editor/monaco` now instead.
export {
  KIRA_EDITOR_THEME,
  loadMonaco,
  type MonacoModule,
  overflowWidgetsContainer,
} from '@workbench/editor/monaco';

import type { MonacoModule } from '@workbench/editor/monaco';

// §9.3/C6 D6: one model per open file tab, keyed by a stable `kira-repo://<repoId>/<path>` URI —
// disposed through the tab kind's existing dropResources hook (closeTab already blind-calls it for
// every registered kind), so model disposal needs no new lifecycle.
const modelCache = new Map<string, import('monaco-editor').editor.ITextModel>();

// C6 D6: built with `Uri.from`, not string interpolation — a path containing a space, '#', '?' or
// '%' does not survive a plain template-literal round trip. `Uri.from` escapes correctly and
// `uri.authority`/`uri.path` give the decoded values straight back.
function repoFileUriObject(
  mod: MonacoModule,
  repoId: string,
  path: string,
): import('monaco-editor').Uri {
  return mod.Uri.from({ scheme: 'kira-repo', authority: repoId, path: `/${path}` });
}

export function repoFileUri(mod: MonacoModule, repoId: string, path: string): string {
  return repoFileUriObject(mod, repoId, path).toString();
}

// A file of a task branch's worktree: keyed by branch id, never by repo, so two tasks on one repo
// never share a model.
export function taskFileUri(mod: MonacoModule, branchId: string, path: string): string {
  return mod.Uri.from({ scheme: 'kira-task', authority: branchId, path: `/${path}` }).toString();
}

// An externally opened file outside any repository: its absolute path under the file scheme.
export function localFileUri(mod: MonacoModule, path: string): string {
  return mod.Uri.file(path).toString();
}

// P74 §7.3: a repo-file tab pinned to a revision (`repoFileTabStateSchema.rev`) — keyed by `rev`
// exactly as repoRevisionDiffUris keys its own two sides below, so a historical read of `path`
// never collides with the live worktree model already cached at repoFileUri's own URI.
export function repoRevisionFileUri(
  mod: MonacoModule,
  repoId: string,
  path: string,
  rev: string,
): string {
  return mod.Uri.from({
    scheme: 'kira-repo',
    authority: repoId,
    path: `/${path}`,
    query: `rev=${rev}`,
  }).toString();
}

// P107 I2-20: repoDiffUris/repoRevisionDiffUris both build the same two Uri.from calls, differing
// only in the query strings — shared here, keyed positionally (not by field name) since the two
// callers return differently-named pairs (head/worktree vs left/right).
function repoUriPair(
  mod: MonacoModule,
  repoId: string,
  path: string,
  queryA: string,
  queryB: string,
): [import('monaco-editor').Uri, import('monaco-editor').Uri] {
  return [
    mod.Uri.from({ scheme: 'kira-repo', authority: repoId, path: `/${path}`, query: queryA }),
    mod.Uri.from({ scheme: 'kira-repo', authority: repoId, path: `/${path}`, query: queryB }),
  ];
}

// C6 §8.3: the diff editor's own two sides share one scheme with the file viewer (so navigation's
// one selector covers both) but need distinct model identities — `query` tells them apart without
// a second scheme.
export function repoDiffUris(
  mod: MonacoModule,
  repoId: string,
  path: string,
): { head: import('monaco-editor').Uri; worktree: import('monaco-editor').Uri } {
  const [head, worktree] = repoUriPair(mod, repoId, path, 'side=head', 'side=worktree');
  return { head, worktree };
}

// C10 §6.1: a commit diff's own two sides — keyed by revision, not by "head"/"worktree", since two
// different commits' diffs of the same path must never collide on one cached model (getOrCreateModel
// returns whatever is already cached at a URI, ignoring the text it was just handed on a cache
// hit). `left`/`right` are the tab's own revision-pair fields (repoDiffTabStateSchema, S8) —
// already the exact strings a commit sha, or (for a root commit) the well-known empty-tree sha.
export function repoRevisionDiffUris(
  mod: MonacoModule,
  repoId: string,
  path: string,
  left: string,
  right: string,
): { left: import('monaco-editor').Uri; right: import('monaco-editor').Uri } {
  const [leftUri, rightUri] = repoUriPair(mod, repoId, path, `rev=${left}`, `rev=${right}`);
  return { left: leftUri, right: rightUri };
}

/** The cached, live model at `uri`, if any; never creates one. */
export function peekModel(uri: string): import('monaco-editor').editor.ITextModel | undefined {
  const model = modelCache.get(uri);
  return model && !model.isDisposed() ? model : undefined;
}

export function getOrCreateModel(
  mod: MonacoModule,
  uri: string,
  text: string,
  language: string,
): import('monaco-editor').editor.ITextModel {
  const existing = modelCache.get(uri);
  if (existing && !existing.isDisposed()) {
    // P79 review fix (Functional, MEDIUM): a cache hit used to hand back whatever content the
    // model already held, silently ignoring the fresh text the caller just read. A cached model can
    // outlive an external change (a `git pull`) with nothing else to invalidate it, so reopening the
    // same URI must not resurrect stale bytes. setValue only touches the model when content actually
    // differs, so the common case (nothing changed) is a cheap compare.
    if (existing.getValue() !== text) existing.setValue(text);
    return existing;
  }
  const model = mod.editor.createModel(text, language, mod.Uri.parse(uri));
  modelCache.set(uri, model);
  return model;
}

export function disposeModel(uri: string): void {
  const model = modelCache.get(uri);
  if (!model) return;
  modelCache.delete(uri);
  if (!model.isDisposed()) model.dispose();
}
