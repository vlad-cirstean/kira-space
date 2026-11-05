import { t } from '@theme/i18n';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { defineStore } from 'pinia';
import { ref } from 'vue';
import { useAdeBoardUiStore } from './adeBoardUi';

export type WorkflowsTab = 'statuses' | 'steps' | 'workflows';

// Workflows page state, in memory only.
export const useAdeWorkflowsUiStore = defineStore('adeWorkflowsUi', () => {
  /** Selected file name (`null` = first listed). */
  const workflowFile = ref<string | null>(null);
  const workflowMode = ref<'graph' | 'yaml'>('graph');
  /** Selected library step file name (`null` = first listed). */
  const stepFile = ref<string | null>(null);
  const stepMode = ref<'form' | 'yaml'>('form');
  /** Active tab of the Workflows page. */
  const tab = ref<WorkflowsTab>('workflows');
  /** The open editor holds edits that are not saved. */
  const dirty = ref(false);

  /** Resolves true when the caller may leave the editor: nothing unsaved, or the user discards it. */
  async function leave(): Promise<boolean> {
    if (!dirty.value) return true;
    const discard = await useConfirmDialogStore().confirm({
      title: t('space.ade.workflowDiscardConfirm.title'),
      message: t('space.ade.workflowDiscardConfirm.body'),
      confirmLabel: t('common.action.discard.label'),
      tone: 'danger',
    });
    if (discard) dirty.value = false;
    return discard;
  }

  /** Opens the Workflows page on `fileName`. */
  function openWorkflow(fileName: string | null): void {
    tab.value = 'workflows';
    workflowFile.value = fileName;
    useAdeBoardUiStore().view = 'workflows';
  }

  /** Opens the Steps tab on library step `fileName`. */
  function openStep(fileName: string | null): void {
    tab.value = 'steps';
    stepFile.value = fileName;
    stepMode.value = 'form';
    useAdeBoardUiStore().view = 'workflows';
  }

  /** Switches tab; false when the user kept unsaved edits. */
  async function setTab(next: WorkflowsTab): Promise<boolean> {
    if (next === tab.value) return true;
    if (!(await leave())) return false;
    tab.value = next;
    return true;
  }

  /** Opens the Workflows page on the task statuses editor; false when the user kept unsaved edits. */
  async function openStatuses(): Promise<boolean> {
    if (!(await leave())) return false;
    tab.value = 'statuses';
    useAdeBoardUiStore().view = 'workflows';
    return true;
  }

  return {
    workflowFile,
    workflowMode,
    stepFile,
    stepMode,
    tab,
    dirty,
    leave,
    openWorkflow,
    openStep,
    setTab,
    openStatuses,
  };
});
