// RepoDiffView.vue's own content-resolution + editor-construction body, extracted so
// RepoMultiDiffView.vue's own per-file sections can share one implementation instead of a second
// copy — CLAUDE.md's "one term per concept" rule. Registered under `editorKey` (editors.ts's own
// registry key, an opaque string) rather than a plain tabId: a single-file diff (RepoDiffView.vue)
// passes its tab id unchanged; one section of a multi-diff
// tab (RepoMultiDiffView.vue) passes `${tabId}:${path}`, since one multi-diff tab registers
// several model pairs under the one tab id.
//
// Deliberately NOT here: `view.find`/`repo.goToFileFromDiff` command registration and review-
// decorations wiring (attachReviewDecorations) — both are single-active-view concerns RepoDiffView
// still owns; registering either once per expanded multi-diff section would mean the last section
// mounted wins, silently shadowing the others'.

import type { DiffSide, DiffSideRead } from '@shared/domain/repo';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { logWarn } from '@workbench/bridge/log';
import { queryClient } from '@workbench/state/queryClient';
import { type Ref, ref, shallowRef, type WatchStopHandle, watch } from 'vue';
import { reviewWorkingDiffOptions } from '../../ade/v2/review/useReviewWorking';
import { control } from '../../bridge/control';
import { gitCall } from '../../repo/git/gitCall';
import { gitRepoIdFor } from '../../repo/git/hostHandlers';
import { withGitRepoHold } from '../../repo/git/repoHolds';
import { gitTransportFor } from '../../repo/git/transport';
import { openRepoFileTab } from '../../state/repoTabs';
import { useSettingsStore } from '../../state/settings';
import { useDiffLayout } from './diffLayout';
import { knownRead, registerDiffEditor, rememberRead, unmountEditor } from './editors';
import { FULL_COMMIT_ID } from './fileContent';
import { monacoLanguageFor } from './language';
import {
  getOrCreateModel,
  KIRA_EDITOR_THEME,
  loadMonaco,
  overflowWidgetsContainer,
  peekModel,
  repoDiffUris,
  repoRevisionDiffUris,
} from './monaco';

// C10 §6.1: file.read's own four-way result, reshaped into DiffSide — the same classification
// codeWorkspaceReadDiff's own two sides already use, so the binary/tooLarge/bothMissing branches
// below and the Monaco model creation stay one implementation for both kinds of diff.
function toDiffSide(result: {
  readonly kind: 'found' | 'missing' | 'binary' | 'tooLarge';
  readonly content?: string;
  readonly bytes?: number;
  readonly limitBytes?: number;
}): DiffSide {
  return {
    kind: result.kind,
    text: result.content ?? '',
    bytes: result.bytes ?? 0,
    limitBytes: result.limitBytes ?? 0,
  };
}

interface DiffSidesLoaded {
  diff: { head: DiffSide; worktree: DiffSide; contentHash?: string };
  gitRepoId: string | undefined;
}

type TextModel = import('monaco-editor').editor.ITextModel;

/** The models a previous mount left behind, for this diff's two sides. */
interface PriorModels {
  head: TextModel;
  worktree: TextModel;
}

function sideFromModel(model: TextModel, extra: Partial<DiffSide>): DiffSide {
  return { kind: 'found', text: model.getValue(), bytes: 0, limitBytes: 0, ...extra };
}

/** A side as read, or the prior model's text when the backend said it had not changed. */
function settleSide(
  side: DiffSideRead,
  model: TextModel | undefined,
  extra: Partial<DiffSide>,
): DiffSide | null {
  if (side.kind !== 'unchanged') return side;
  return model ? sideFromModel(model, extra) : null;
}

// mount()'s own two-branch content read (worktree HEAD-vs-worktree vs a
// revision-pinned commit diff), each with its own try/catch/error-message plumbing — lifted out
// so mount() itself only branches on the result. `error` is a string, not thrown: mount()'s own
// contract is to set `state`/`errorMessage` and return, never to unwind past this call.
async function loadDiffSides(
  repoId: string,
  path: string,
  left: string | null,
  right: string | null,
  reviewBranch: string | undefined,
  workingBranchId: string | undefined,
  prior: PriorModels | undefined,
  editorKey: string,
): Promise<DiffSidesLoaded | { error: string }> {
  if (workingBranchId !== undefined) {
    try {
      const diff = await queryClient.fetchQuery(reviewWorkingDiffOptions(workingBranchId, path));
      return { diff, gitRepoId: undefined };
    } catch (err) {
      return { error: errorText(err) };
    }
  }
  if (left === null) {
    try {
      const known = prior
        ? knownRead(editorKey, [prior.head.getVersionId(), prior.worktree.getVersionId()])
        : null;
      const gate = known?.stat ? { ifStat: known.stat, ifHeadOid: known.headOid } : undefined;
      const diff = await control.codeWorkspaceReadDiff(repoId, path, gate);
      const head = settleSide(diff.head, prior?.head, { oid: known?.headOid });
      const worktree = settleSide(diff.worktree, prior?.worktree, { stat: known?.stat });
      // The gate is only sent with prior models, so an unchanged side always has one to reuse.
      if (!head || !worktree) return { error: t('space.repo.view.repoNotOpen.error') };
      return { diff: { ...diff, head, worktree }, gitRepoId: undefined };
    } catch (err) {
      return { error: errorText(err) };
    }
  }

  const gitRepoId = gitRepoIdFor(repoId);
  if (!gitRepoId || right === null) {
    return { error: t('space.repo.view.repoNotOpen.error') };
  }
  // Both sides are commits, which never change: the models from the last mount are current.
  if (
    prior &&
    reviewBranch === undefined &&
    FULL_COMMIT_ID.test(left) &&
    FULL_COMMIT_ID.test(right)
  ) {
    return {
      diff: { head: sideFromModel(prior.head, {}), worktree: sideFromModel(prior.worktree, {}) },
      gitRepoId,
    };
  }
  try {
    const [leftResult, rightResult, snapshot] = await withGitRepoHold(gitRepoId, () =>
      Promise.all([
        gitCall('file.read', { repoId: gitRepoId, rev: left, path }),
        gitCall('file.read', { repoId: gitRepoId, rev: right, path }),
        reviewBranch === undefined
          ? null
          : gitCall('review.snapshot', { repoId: gitRepoId, branch: reviewBranch, path }),
      ]),
    );
    // A since-review left side is the bytes the reviewer saw, which survive a rewritten branch
    // that made the reviewed commit unreachable.
    const head =
      snapshot?.kind === 'text' && snapshot.text !== null && snapshot.reviewedAtSha === left
        ? toDiffSide({ kind: 'found', content: snapshot.text, bytes: snapshot.text.length })
        : toDiffSide(leftResult);
    return { diff: { head, worktree: toDiffSide(rightResult) }, gitRepoId };
  } catch (err) {
    return { error: errorText(err) };
  }
}

// The diff editor's own "go to file" — a revision-backed diff resolves the target through the git
// transport; the plain worktree-vs-HEAD comparison already shows the live file on disk
// (right === null), so the modified pane *is* that file — open it
// directly, no round trip.
function wireGoToFile(
  modifiedEditor: import('monaco-editor').editor.IStandaloneCodeEditor,
  repoId: string,
  path: string,
  gitRepoId: string | undefined,
  right: string | null,
): () => void {
  return () => {
    const line = modifiedEditor.getPosition()?.lineNumber ?? 1;
    if (gitRepoId === undefined || right === null) {
      openRepoFileTab(repoId, path, { preview: false, reveal: { line } });
      return;
    }
    const targetGitRepoId = gitRepoId;
    const targetRev = right;
    const transport = gitTransportFor(repoId);
    transport
      .request('editor.goToFile', { repoId: targetGitRepoId, rev: targetRev, path, line })
      .then((outcome) => {
        // `liveFile`/`virtualBlob` already opened their own tab (hostHandlers.ts's own
        // composition) — the tab switch is the visible confirmation. `unavailable` is the one
        // branch with no other signal; this raw editor view has no toast/live-region channel to
        // surface it through, so it goes to the console rather than dropping silently.
        if (outcome.kind === 'unavailable') {
          logWarn('repo.goToFileFromDiff', `${path} unavailable — ${outcome.reason}`);
        }
      })
      .catch((err: unknown) => {
        logWarn('repo.goToFileFromDiff', 'failed', err);
      })
      .finally(() => transport.dispose());
  };
}

type DiffEditorState = 'loading' | 'found' | 'binary' | 'tooLarge' | 'bothMissing' | 'error';

export interface DiffEditorParams {
  /** editors.ts's own registry key — see this file's own header comment. */
  editorKey: string;
  /** Already resolved from the tab's own workspaceId — a missing repository is the caller's own
   *  error state to show (nothing to mount at all), not this composable's. */
  repoId: string;
  path: string;
  /** C10 §6.1/§6.2: both null is C6's original HEAD-vs-worktree comparison; a commit diff always
   *  supplies both (a root commit's left is git's well-known empty-tree sha, never null). */
  left: string | null;
  right: string | null;
  /** Only turns the glyph margin off (§7.3 gotcha 1) — the review object itself,
   *  and attaching its comment-thread layer, stay the caller's own concern. */
  review: boolean;
  /** The reviewed branch of a review diff; the left side then comes from `review.snapshot` when
   *  it stored this revision's text. */
  reviewBranch?: string;
  /** An uncommitted file of this task branch's worktree: both sides come from the backend, and
   *  `left`/`right` only key the Monaco models. */
  workingBranchId?: string;
  /** Checked right before the widget is created; a superseded mount (a section collapsed and
   *  re-expanded mid-read) returns false and leaves nothing to dispose. */
  isCurrent?: () => boolean;
}

export interface DiffEditorHandle {
  state: Ref<DiffEditorState>;
  errorMessage: Ref<string>;
  /** Set once mount() reaches 'found' — RepoDiffView.vue's own view.find/review-decorations
   *  wiring, both single-active-view concerns this composable deliberately stays out of (this
   *  file's own header comment), reach the live editor through it. */
  editor: Ref<import('monaco-editor').editor.IStandaloneDiffEditor | null>;
  /** Hash of the uncommitted worktree text the models were filled from; the caller keeps it in
   *  step when it rewrites them. Undefined for a diff that is not an uncommitted review read. */
  contentHash: Ref<string | undefined>;
  mount(): Promise<void>;
  /** onUnmounted — disposes the widget only; the model(s) survive in editors.ts (unmountEditor's
   *  own contract) until dropResources actually closes the tab. */
  dispose(): void;
  /** A no-op before mount() reaches 'found' or after dispose(). */
  goToFile(): void;
}

// Read-only, unchanged from the file viewer — readOnly/domReadOnly block the keyboard and
// paste; renderMarginRevertIcon/renderGutterMenu are the second layer, hiding the revert/apply
// affordances that would otherwise let a user trigger a write from a widget built for the
// extension's read-write use (not cosmetic).
export function useDiffEditor(
  container: Ref<HTMLElement | null>,
  params: DiffEditorParams,
): DiffEditorHandle {
  const state = ref<DiffEditorState>('loading');
  const errorMessage = ref('');
  const contentHash = ref<string | undefined>();
  const editor = shallowRef<import('monaco-editor').editor.IStandaloneDiffEditor | null>(null);
  let goToFileImpl: (() => void) | null = null;
  let stopLayoutWatch: WatchStopHandle | null = null;

  async function mount(): Promise<void> {
    const settingsStore = useSettingsStore();
    const { editorKey, repoId, path, left, right, review, reviewBranch, workingBranchId } = params;
    const mod = await loadMonaco();
    let headUri: import('monaco-editor').Uri;
    let worktreeUri: import('monaco-editor').Uri;
    if (left === null) {
      ({ head: headUri, worktree: worktreeUri } = repoDiffUris(mod, repoId, path));
    } else {
      ({ left: headUri, right: worktreeUri } = repoRevisionDiffUris(
        mod,
        repoId,
        path,
        left,
        // biome-ignore lint/style/noNonNullAssertion: checked alongside left above — both null or both set.
        right!,
      ));
    }
    const priorHead = peekModel(headUri.toString());
    const priorWorktree = peekModel(worktreeUri.toString());
    const loaded = await loadDiffSides(
      repoId,
      path,
      left,
      right,
      reviewBranch,
      workingBranchId,
      priorHead && priorWorktree ? { head: priorHead, worktree: priorWorktree } : undefined,
      editorKey,
    );
    if ('error' in loaded) {
      state.value = 'error';
      errorMessage.value = loaded.error;
      return;
    }
    const { diff, gitRepoId } = loaded;

    if (diff.head.kind === 'binary' || diff.worktree.kind === 'binary') {
      state.value = 'binary';
      return;
    }
    if (diff.head.kind === 'tooLarge' || diff.worktree.kind === 'tooLarge') {
      state.value = 'tooLarge';
      return;
    }
    if (diff.head.kind === 'missing' && diff.worktree.kind === 'missing') {
      state.value = 'bothMissing';
      return;
    }

    const layout = useDiffLayout();
    // Unmounted (tab closed/switched away, or the section collapsed) while the read/import above
    // was in flight.
    if (!container.value || params.isCurrent?.() === false) return;

    const language = monacoLanguageFor(path);
    const original = getOrCreateModel(mod, headUri.toString(), diff.head.text, language);
    const modified = getOrCreateModel(mod, worktreeUri.toString(), diff.worktree.text, language);
    contentHash.value = diff.contentHash;

    const created = mod.editor.createDiffEditor(container.value, {
      theme: KIRA_EDITOR_THEME,
      readOnly: true,
      domReadOnly: true,
      originalEditable: false,
      renderMarginRevertIcon: false,
      renderGutterMenu: false,
      automaticLayout: true,
      renderSideBySide: layout.value === 'sideBySide',
      // A narrow pane (the review window centre) must not flip to inline.
      useInlineViewWhenSpaceIsLimited: false,
      ignoreTrimWhitespace: false,
      hideUnchangedRegions: {
        enabled: true,
        contextLineCount: 3,
        minimumLineCount: 3,
        revealLineCount: 20,
      },
      // §7.3 gotcha 1: the registered option default and the .d.ts prose disagree — set explicitly
      // (off: the review marks live in the line numbers and hunk rows) rather than depend on either being right in a future Monaco bump.
      // The plain diff passes nothing, unchanged from before this phase.
      ...(review ? { glyphMargin: false } : {}),
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
      fontFamily: settingsStore.appearance.fontFamily,
      fontSize: settingsStore.appearance.fontSize,
      // editor/monaco.ts's own overflowWidgetsContainer doc comment — same reasoning as
      // RepoFileView.vue's own plain editor.
      fixedOverflowWidgets: true,
      overflowWidgetsDomNode: overflowWidgetsContainer(),
    });
    // Outside any component scope (mount() is async), so the stop handle is kept for dispose().
    stopLayoutWatch = watch(layout, (value) => {
      created.updateOptions({ renderSideBySide: value === 'sideBySide' });
    });
    created.setModel({ original, modified });
    registerDiffEditor(editorKey, [headUri.toString(), worktreeUri.toString()], created);
    if (left === null && diff.worktree.stat) {
      rememberRead(editorKey, {
        stat: diff.worktree.stat,
        headOid: diff.head.oid,
        versionIds: [original.getVersionId(), modified.getVersionId()],
      });
    }
    editor.value = created;

    // C7 D13/§6: the modified (worktree) pane, never the HEAD pane — it's the side whose content
    // matches the file on disk (C6 D7's own reasoning for which side is navigable), and
    // IStandaloneDiffEditor itself has no getAction, so this has to reach through to one pane.
    const modifiedEditor = created.getModifiedEditor();
    // The worktree file is not in the repo's main checkout, which is what the opener would show.
    goToFileImpl =
      workingBranchId === undefined
        ? wireGoToFile(modifiedEditor, repoId, path, gitRepoId, right)
        : null;

    state.value = 'found';
  }

  function dispose(): void {
    goToFileImpl = null;
    stopLayoutWatch?.();
    stopLayoutWatch = null;
    editor.value = null;
    unmountEditor(params.editorKey);
  }

  function goToFile(): void {
    goToFileImpl?.();
  }

  return { state, errorMessage, editor, contentHash, mount, dispose, goToFile };
}
