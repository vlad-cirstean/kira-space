import { t } from '@theme/i18n';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { defineStore } from 'pinia';
import { ref } from 'vue';
import { useModeStore } from '../../../state/mode';
import { switchAdeView } from './adeView';

export type ReposTab = 'repos' | 'folders' | 'files';

// Repositories page state and its leave guard, in memory only.
export const useAdeReposUiStore = defineStore('adeReposUi', () => {
  const tab = ref<ReposTab>('repos');
  /** Selected repo (`null` = first listed). */
  const selectedRepoId = ref<string | null>(null);
  /** The open settings file holds edits that are not saved. */
  const dirty = ref(false);

  /** Resolves true when the caller may leave the file: nothing unsaved, or the user discards it. */
  async function leave(): Promise<boolean> {
    if (!dirty.value) return true;
    const discard = await useConfirmDialogStore().confirm({
      title: t('space.ade.workflowDiscardConfirm.title'),
      message: t('space.ade.repos.discardConfirm.body'),
      confirmLabel: t('common.action.discard.label'),
      tone: 'danger',
    });
    if (discard) dirty.value = false;
    return discard;
  }

  async function setTab(next: ReposTab): Promise<void> {
    if (next === tab.value) return;
    if (!(await leave())) return;
    tab.value = next;
  }

  async function pickRepo(id: string): Promise<void> {
    if (id === selectedRepoId.value && tab.value === 'repos') return;
    if (!(await leave())) return;
    selectedRepoId.value = id;
    tab.value = 'repos';
  }

  /** Opens the Repositories page, on `repoId` or `tab` when given. */
  async function open(opts: { repoId?: string; tab?: ReposTab } = {}): Promise<void> {
    useModeStore().setMode('ade');
    if (!(await switchAdeView('repos'))) return;
    // switchAdeView skips the guard when the page is already open, so guard a change of file here.
    const nextTab = opts.tab ?? 'repos';
    const changes =
      (opts.repoId !== undefined && opts.repoId !== selectedRepoId.value) || nextTab !== tab.value;
    if (changes && !(await leave())) return;
    if (opts.repoId) selectedRepoId.value = opts.repoId;
    tab.value = nextTab;
  }

  return { tab, selectedRepoId, dirty, leave, setTab, pickRepo, open };
});
