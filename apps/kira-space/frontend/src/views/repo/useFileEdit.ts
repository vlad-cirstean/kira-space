import type { EditContent } from '@shared/domain/repo';
import { useMutation } from '@tanstack/vue-query';
import { t } from '@theme/i18n';
import { useWindowFocus } from '@vueuse/core';
import { useTaskAction } from '@workbench/behaviour/asyncAction';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, type Ref, watch } from 'vue';
import { control } from '../../bridge/control';
import { useFileTreeStore } from '../../repo/state/fileTree';
import { useFileEditStore } from '../../state/fileEdit';
import { useTabsStore } from '../../state/tabs';
import type { MonacoModule } from './monaco';
import { EDIT_OPTIONS, READ_OPTIONS } from './useReadonlyFileViewer';

type StandaloneEditor = import('monaco-editor').editor.IStandaloneCodeEditor;
type Model = import('monaco-editor').editor.ITextModel;

export interface FileEditOptions {
  tabId: string;
  path: string;
  /** Element the Save chord is scoped to. */
  root: Ref<HTMLElement | null>;
  repoId: () => string | null;
  /** Markdown files edit as Source: called on entering edit mode. */
  showSource: () => void;
  /** The text of the buffer left edit mode (feeds the markdown Reading render). */
  onText: (text: string) => void;
  /** Blame line numbers stop matching once the text changes: dropped on enter, restored on leave. */
  dropBlame: () => void;
  restoreBlame: () => void;
  /** The file is gone from disk and the user chose to drop the editor. */
  onGone: () => void;
}

/** Edit mode of one mounted file view: Monaco option switching, the hash-guarded save, the discard
 *  prompt, and the on-focus re-check of the file on disk. State that must survive a tab switch
 *  (base hash, dirty flag, conflict) lives in `useFileEditStore`; the draft itself in the cached
 *  Monaco model. */
export function useFileEdit(o: FileEditOptions) {
  const store = useFileEditStore();
  const tabsStore = useTabsStore();
  const confirmDialog = useConfirmDialogStore();
  const fileTree = useFileTreeStore();
  const readMutation = useMutation({
    mutationFn: (repoId: string) => control.codeWorkspaceReadFileForEdit(repoId, o.path),
  });
  const writeMutation = useMutation({
    mutationFn: (a: { repoId: string; text: string; baseHash: string }) =>
      control.codeWorkspaceWriteFile(a.repoId, o.path, a.text, a.baseHash),
  });

  let editor: StandaloneEditor | null = null;
  const NO_SUGGEST_ACTION = 'kira.noSuggest'; // doubles as the required label; never shown
  let contentSub: { dispose(): void } | null = null;
  let noSuggestSub: { dispose(): void } | null = null;
  const surface = useTaskAction();
  const errorMessage = surface.error;

  const editing = computed(() => store.isEditing(o.tabId));
  const dirty = computed(() => store.isDirty(o.tabId));
  const conflict = computed(() => store.sessions[o.tabId]?.conflict ?? null);
  const saving = computed(() => writeMutation.isPending.value);
  const busy = computed(() => saving.value || readMutation.isPending.value);

  function model(): Model | null {
    return editor?.getModel() ?? null;
  }

  function trackDirty(): void {
    contentSub?.dispose();
    const m = model();
    contentSub =
      m?.onDidChangeContent(() => {
        const s = store.sessions[o.tabId];
        if (s) store.setDirty(o.tabId, m.getAlternativeVersionId() !== s.savedVersionId);
      }) ?? null;
  }

  function failureText(c: EditContent): string {
    switch (c.kind) {
      case 'notUtf8':
        return t('space.repo.file.notUtf8.error');
      case 'binary':
        return t('space.repo.view.binaryPreview.empty');
      case 'tooLarge':
        return t('space.repo.view.tooLargePreview.empty');
      default:
        return t('space.repo.view.missing.empty');
    }
  }

  async function readFresh(): Promise<EditContent | null> {
    const repoId = o.repoId();
    if (!repoId) return null;
    try {
      return await readMutation.mutateAsync(repoId);
    } catch (err) {
      surface.fail(err);
      return null;
    }
  }

  /** Replaces the buffer with `text` as the new clean, saved state. */
  function syncBuffer(text: string, hash: string): void {
    const m = model();
    if (!m) return;
    if (m.getValue() !== text) m.setValue(text);
    store.markSaved(o.tabId, hash, m.getAlternativeVersionId());
  }

  function enterOptions(): void {
    editor?.updateOptions(EDIT_OPTIONS);
    trackDirty();
  }

  /** Called by the view once its editor exists. Re-applies edit options when the tab was already
   *  editing before a tab switch, then re-checks the file on disk. */
  function attach(e: StandaloneEditor, mod: MonacoModule): void {
    editor = e;
    // Manual completion (Ctrl+Space, Cmd+I) would still open a "No suggestions" widget; a no-op
    // action on the same chord shadows the built-in binding. `addAction` is disposable and editor
    // scoped; `addCommand` is global and its handler would pin this closure forever.
    const { KeyMod, KeyCode } = mod;
    noSuggestSub?.dispose();
    noSuggestSub = e.addAction({
      id: NO_SUGGEST_ACTION,
      label: NO_SUGGEST_ACTION,
      keybindings: [
        KeyMod.CtrlCmd | KeyCode.Space,
        KeyMod.WinCtrl | KeyCode.Space,
        KeyMod.CtrlCmd | KeyCode.KeyI,
      ],
      run: () => {},
    });
    if (!store.isEditing(o.tabId)) return;
    enterOptions();
    void checkDisk();
  }

  function detach(): void {
    contentSub?.dispose();
    contentSub = null;
    noSuggestSub?.dispose();
    noSuggestSub = null;
    editor = null;
  }

  async function enter(): Promise<void> {
    surface.reset();
    if (!editor || editing.value || busy.value) return;
    const fresh = await readFresh();
    const m = model();
    if (!fresh || !m) return;
    if (fresh.kind !== 'found') {
      surface.fail(failureText(fresh));
      return;
    }
    if (m.getValue() !== fresh.text) m.setValue(fresh.text);
    // A preview tab is evicted by the next single-click open, taking the draft with it.
    tabsStore.promoteTab(o.tabId);
    o.showSource();
    o.dropBlame();
    store.begin(o.tabId, fresh.hash, m.getAlternativeVersionId());
    enterOptions();
    editor.focus();
  }

  function leave(): void {
    contentSub?.dispose();
    contentSub = null;
    editor?.updateOptions(READ_OPTIONS);
    store.end(o.tabId);
    const m = model();
    if (m) o.onText(m.getValue());
    o.restoreBlame();
  }

  async function persist(force: boolean): Promise<void> {
    const repoId = o.repoId();
    const m = model();
    const session = store.sessions[o.tabId];
    if (!repoId || !m || !session || saving.value) return;
    if (!force && !session.dirty) return;
    const versionId = m.getAlternativeVersionId();
    // preserveBOM: Monaco strips a leading BOM from the model; without it a save would drop it.
    const text = m.getValue(undefined, true);
    surface.reset();
    try {
      const res = await writeMutation.mutateAsync({ repoId, text, baseHash: session.baseHash });
      store.markSaved(o.tabId, res.hash, versionId);
      // The user may have typed while the write ran.
      store.setDirty(o.tabId, m.getAlternativeVersionId() !== versionId);
      if (fileTree.isRepoTreeLoaded(repoId)) void fileTree.refreshRepoTree(repoId);
    } catch (err) {
      if ((err as { code?: string }).code === 'E_CONFLICT') {
        const probe = await readFresh();
        store.setConflict(o.tabId, probe?.kind === 'missing' ? 'gone' : 'changed');
        return;
      }
      surface.fail(err);
    }
  }

  const save = () => persist(false);

  /** Takes the file's current hash as the new base, then saves the buffer over it. */
  async function overwrite(): Promise<void> {
    const fresh = await readFresh();
    if (!fresh) return;
    if (fresh.kind !== 'found' && fresh.kind !== 'missing') {
      surface.fail(failureText(fresh));
      return;
    }
    const s = store.sessions[o.tabId];
    if (!s) return;
    s.baseHash = fresh.hash;
    s.conflict = null;
    await persist(true);
  }

  /** Discards the buffer in favour of the file on disk, staying in edit mode. */
  async function reload(): Promise<void> {
    const fresh = await readFresh();
    if (!fresh) return;
    if (fresh.kind === 'missing') {
      leave();
      o.onGone();
      return;
    }
    if (fresh.kind !== 'found') {
      surface.fail(failureText(fresh));
      return;
    }
    syncBuffer(fresh.text, fresh.hash);
    surface.reset();
  }

  async function done(): Promise<void> {
    if (!editing.value) return;
    if (!dirty.value) {
      leave();
      return;
    }
    const ok = await confirmDialog.confirm({
      title: t('common.tab.discard.title'),
      message: t('common.tab.discard.body', { count: 1 }),
      confirmLabel: t('common.action.discard.label'),
      tone: 'danger',
    });
    if (!ok) return;
    const fresh = await readFresh();
    if (!fresh) return;
    if (fresh.kind === 'found') {
      syncBuffer(fresh.text, fresh.hash);
      leave();
    } else if (fresh.kind === 'missing') {
      leave();
      o.onGone();
    } else {
      surface.fail(failureText(fresh));
    }
  }

  /** The file may have changed under the editor (another app, an agent): a clean buffer follows
   *  the disk silently, a dirty one raises the conflict now instead of at Save. */
  async function checkDisk(): Promise<void> {
    if (!store.isEditing(o.tabId) || busy.value) return;
    const fresh = await readFresh();
    const s = store.sessions[o.tabId];
    if (!fresh || !s) return;
    if (fresh.kind === 'missing') {
      store.setConflict(o.tabId, 'gone');
    } else if (fresh.kind !== 'found') {
      store.setConflict(o.tabId, 'changed');
    } else if (fresh.hash !== s.baseHash) {
      if (s.dirty) store.setConflict(o.tabId, 'changed');
      else syncBuffer(fresh.text, fresh.hash);
    }
  }

  const focused = useWindowFocus();
  watch(focused, (now) => {
    if (now) void checkDisk();
  });

  useShortcut(o.root, ['form.save'], () => void save(), {
    capture: true,
    when: () => store.isEditing(o.tabId),
  });

  return {
    editing,
    dirty,
    conflict,
    saving,
    busy,
    errorMessage,
    attach,
    detach,
    enter,
    save,
    done,
    reload,
    overwrite,
  };
}
