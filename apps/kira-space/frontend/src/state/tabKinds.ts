import { shortSha } from '@kira/git-core';
import type { PaletteColor } from '@shared/domain/color';
import { t } from '@theme/i18n';
import { scriptRunState } from '@workbench/automations/runs/runStates';
import type { MenuItem } from '@workbench/state/contextMenu';
import { scriptRunTabKind } from '@workbench/tabs/scriptRunTabKind';
import { terminalTabKind } from '@workbench/tabs/terminalTabKind';
import { parseStateWith, type TabKindRegistry } from '@workbench/tabs/types';
import { useRepoLinksStore } from '../repo/state/repoLinks';
import { STUDIO_TAB_KINDS } from '../studio/state/tabKinds';
import {
  dropLocalFileTab,
  dropRepoDiffTab,
  dropRepoFileTab,
  dropRepoMultiDiffTab,
} from '../views/repo/editors';
import { useCodeReposStore } from './coderepos';
import { useFileEditStore } from './fileEdit';
import {
  asRepoFileTab,
  defaultLocalFileTabState,
  defaultRepoDiffTabState,
  defaultRepoFileTabState,
  defaultRepoGraphTabState,
  defaultRepoMultiDiffTabState,
  type LocalFileTabRecord,
  type LocalFileTabState,
  localFileTabStateSchema,
  type RepoDiffTabRecord,
  type RepoDiffTabState,
  type RepoFileTabRecord,
  type RepoFileTabState,
  type RepoGraphTabRecord,
  type RepoGraphTabState,
  type RepoMultiDiffTabRecord,
  type RepoMultiDiffTabState,
  repoDiffTabStateSchema,
  repoFileTabStateSchema,
  repoGraphTabStateSchema,
  repoMultiDiffTabStateSchema,
  SPACE_TAB_KIND_MODE,
  type SpaceTabKind,
  type TabKind,
  type TabRecord,
} from './tabDomain';
import { useTerminalsStore } from './terminals';

// P100 Part 2: kira-space's own tab-kind registry — the same split-from-workbench/tabViews.ts
// shape Kira Studio's state/tabKinds.ts uses (F19, that file's own header comment), trimmed to
// this app's own five kinds only (repo-graph/repo-file/repo-diff/repo-multi-diff/terminal). Every
// entry below is Kira Studio's own behaviour verbatim; only the workspace-key lookups changed,
// since this app's TabRecord.workspaceId IS the bare repoId (no `repo:` prefix, no
// repoIdOfWorkspace indirection — state/workspace.ts's own header comment), and railColor uses
// PaletteColor directly (there is no Connections concept here to own a ConnectionColor alias).
// P103 Part 2 (§5.1): `SpaceTabKind` now lives in state/tabDomain.ts (this app's own kind union,
// split out of the old shared `packages/shared/domain/tabs.ts`), and `TabKindDef` is
// `@workbench/tabs/types`'s generic contract, instantiated below via `TabKindRegistry`.

/** A codicon name, or a file path whose icon comes from the shared seti set
 *  (`repo/fileIcon.ts`) — the same rule the repo file tree and the diff tree already use. */
type TabIcon = string | { readonly filePath: string };

function noDrop(): void {
  // repo-graph has no page store of its own — nothing to free.
}

// A repo-file tab's `path` is a plain repository-relative path ('src/main.go'), not an encoded
// NodePath, so this kind gets its own basename-only title rather than pathTail's "kind:name"
// parsing. Also used for a repo-diff tab's own title (repoDiffTitle below), which has no `rev`
// field — asRepoFileTab returns null there, so the suffix only appears for an actual repo-file tab
// pinned to a revision.
function repoFileTitle(tab: TabRecord): string {
  const idx = tab.path.lastIndexOf('/');
  const base = idx < 0 ? tab.path : tab.path.slice(idx + 1);
  const rev = asRepoFileTab(tab)?.state.rev;
  return rev == null ? base : `${base} (${shortSha(rev)})`;
}

// Same basename-only reasoning as repoFileTitle, plus a suffix distinguishing a diff tab from a
// file tab open on the identical path. A commit diff (revision pair present) is titled with both
// short shas instead, so two commits' diffs of the same file read as genuinely different tabs.
function repoDiffTitle(tab: TabRecord): string {
  const diff = tab as RepoDiffTabRecord;
  if (diff.state.left !== null && diff.state.right !== null) {
    const left = diff.state.leftLabel ?? shortSha(diff.state.left);
    const right = diff.state.rightLabel ?? shortSha(diff.state.right);
    return `${repoFileTitle(tab)} (${left} ↔ ${right})`;
  }
  return `${repoFileTitle(tab)} (Working Tree)`;
}

// No `repoFileTitle`-style basename — `path` doesn't exist on this kind (it covers a whole
// commit's file set, not one file), so the title is the revision pair plus a file count.
function repoMultiDiffTitle(tab: TabRecord): string {
  const diff = tab as RepoMultiDiffTabRecord;
  const count = diff.state.files.length;
  return `${diff.state.leftLabel} ↔ ${diff.state.rightLabel} (${count} file${count === 1 ? '' : 's'})`;
}

// A repo tab's rail carries its repository's chosen colour; 'none' keeps the reserved slot empty.
function repoRailColor(tab: TabRecord): PaletteColor | undefined {
  const color = useRepoLinksStore().repoColorOf(tab.workspaceId);
  return color === 'none' ? undefined : color;
}

const SPACE_TAB_KINDS: TabKindRegistry<SpaceTabKind, TabRecord, TabIcon, PaletteColor, MenuItem> = {
  'repo-graph': {
    mode: SPACE_TAB_KIND_MODE['repo-graph'],
    title: (tab) =>
      useCodeReposStore().codeRepoRecord(tab.workspaceId ?? '')?.name ?? t('common.graph.label'),
    icon: () => 'source-control',
    railColor: repoRailColor,
    defaultState: (): RepoGraphTabState => defaultRepoGraphTabState(),
    // Never actually reached (`pinned: true` refuses duplication, state/tabs.ts), but a real
    // default rather than `{}` keeps this consistent with every other kind's own duplicateState.
    duplicateState: (_tab: RepoGraphTabRecord): RepoGraphTabState => defaultRepoGraphTabState(),
    dropResources: noDrop,
    menuExtras: () => [],
    parseState: parseStateWith(repoGraphTabStateSchema),
    pinned: true,
    keepMounted: true,
  },
  // One opened repository file, rendered by Monaco (read-only). The same seti icon the file tree
  // already shows for this path (RepoTreeRow.vue's own fileIconStyle, repo/fileIcon.ts).
  'repo-file': {
    mode: SPACE_TAB_KIND_MODE['repo-file'],
    title: repoFileTitle,
    icon: (tab) => ({ filePath: tab.path }),
    railColor: repoRailColor,
    defaultState: (): RepoFileTabState => defaultRepoFileTabState(),
    duplicateState: (tab: RepoFileTabRecord): RepoFileTabState => ({ ...tab.state }),
    // Disposes the live editor widget (if any) and the cached model — RepoFileView.vue's own
    // unmount (a mere tab switch) never reaches this; only an actual close does.
    dropResources: (tabId) => dropRepoFileTab(tabId),
    menuExtras: () => [],
    parseState: parseStateWith(repoFileTabStateSchema),
    // An edit buffer with unsaved changes: closing the tab asks to discard first.
    isDirty: (tabId) => useFileEditStore().isDirty(tabId),
    badge: (tab) =>
      useFileEditStore().isDirty(tab.id)
        ? { icon: 'circle-filled', tooltip: t('common.state.unsavedChanges.status') }
        : null,
  },
  // An externally opened file outside every imported repository (`path` is absolute): the title
  // is the basename. No rail colour: no repository behind it.
  'local-file': {
    mode: SPACE_TAB_KIND_MODE['local-file'],
    title: repoFileTitle,
    icon: (tab) => ({ filePath: tab.path }),
    railColor: () => undefined,
    defaultState: (): LocalFileTabState => defaultLocalFileTabState(),
    duplicateState: (tab: LocalFileTabRecord): LocalFileTabState => ({ ...tab.state }),
    dropResources: (tabId) => dropLocalFileTab(tabId),
    menuExtras: () => [],
    parseState: parseStateWith(localFileTabStateSchema),
  },
  // A HEAD-vs-worktree diff, opened from "Open changes" — read-only, mirroring 'repo-file''s
  // shape exactly. No badge, no pinned: a read-only tab.
  'repo-diff': {
    mode: SPACE_TAB_KIND_MODE['repo-diff'],
    title: repoDiffTitle,
    icon: () => 'git-compare',
    railColor: repoRailColor,
    defaultState: (): RepoDiffTabState => defaultRepoDiffTabState(),
    // A commit diff's revision pair must survive duplication — resetting to `{}` would silently
    // turn a duplicated commit-diff tab into a HEAD-vs-worktree one.
    duplicateState: (tab: RepoDiffTabRecord): RepoDiffTabState => ({ ...tab.state }),
    dropResources: (tabId) => dropRepoDiffTab(tabId),
    menuExtras: () => [],
    parseState: parseStateWith(repoDiffTabStateSchema),
  },
  // One commit's whole changed-file set, one tab (VS Code's multi-file diff) — read-only,
  // mirroring 'repo-diff''s own shape. No badge, no pinned, same reasoning.
  'repo-multi-diff': {
    mode: SPACE_TAB_KIND_MODE['repo-multi-diff'],
    title: repoMultiDiffTitle,
    icon: () => 'diff-multiple',
    railColor: repoRailColor,
    // Never reached through a generic "new tab of this kind" affordance — openRepoMultiDiffTab
    // always supplies a real files/revision pair. This placeholder only satisfies TabKindDef's own
    // required member.
    defaultState: (): RepoMultiDiffTabState =>
      defaultRepoMultiDiffTabState([], { left: '', right: '', leftLabel: '', rightLabel: '' }),
    // files/review are arrays/objects — copied fresh so editing the duplicate's own state (were it
    // ever mutated in place) can't reach back into the original's.
    duplicateState: (tab: RepoMultiDiffTabRecord): RepoMultiDiffTabState => ({
      ...tab.state,
      files: [...tab.state.files],
      review: tab.state.review ? { ...tab.state.review } : null,
    }),
    dropResources: (tabId) => dropRepoMultiDiffTab(tabId),
    menuExtras: () => [],
    parseState: parseStateWith(repoMultiDiffTabStateSchema),
  },
  // An embedded shell at one worktree's directory, rendered with @xterm/xterm. A launch's own
  // label (a script's name, or 'Claude Code') wins over the cwd's basename.
  // The descriptor itself moved to workbench's own terminalTabKind, shared with kira-studio's
  // identical copy — only the mode constant and dropResources' own store differ per app.
  terminal: terminalTabKind<'terminal', TabRecord, TabIcon, PaletteColor, MenuItem>(
    SPACE_TAB_KIND_MODE.terminal,
    (tabId) => useTerminalsStore().closeTerminalSession(tabId),
    (tabId) => useTerminalsStore().terminalSession(tabId),
  ),
  'script-run': scriptRunTabKind<'script-run', TabRecord, TabIcon, PaletteColor, MenuItem>(
    SPACE_TAB_KIND_MODE['script-run'],
    scriptRunState,
  ),
};

export const TAB_KINDS: TabKindRegistry<TabKind, TabRecord, TabIcon, PaletteColor, MenuItem> = {
  ...SPACE_TAB_KINDS,
  ...STUDIO_TAB_KINDS,
};
