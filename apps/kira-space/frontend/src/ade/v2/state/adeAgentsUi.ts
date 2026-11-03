import { t } from '@theme/i18n';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { defineStore } from 'pinia';
import { ref } from 'vue';

export type AgentsTab = 'context' | 'settings';

// Agent config page state and its leave guard, in memory only.
export const useAdeAgentsUiStore = defineStore('adeAgentsUi', () => {
  const tab = ref<AgentsTab>('context');
  /** The open editor (context or settings) holds edits that are not saved. */
  const dirty = ref(false);

  /** Resolves true when the caller may leave the editor: nothing unsaved, or the user discards it. */
  async function leave(): Promise<boolean> {
    if (!dirty.value) return true;
    const discard = await useConfirmDialogStore().confirm({
      title: t('space.ade.workflowDiscardConfirm.title'),
      message:
        tab.value === 'context'
          ? t('space.ade.context.discardConfirm.body')
          : t('space.ade.settings.discardConfirm.body'),
      confirmLabel: t('common.action.discard.label'),
      tone: 'danger',
    });
    if (discard) dirty.value = false;
    return discard;
  }

  async function openTab(next: AgentsTab): Promise<void> {
    if (next === tab.value) return;
    if (!(await leave())) return;
    tab.value = next;
  }

  return { tab, dirty, leave, openTab };
});
