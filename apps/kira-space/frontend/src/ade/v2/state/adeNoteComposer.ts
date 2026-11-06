import { defineStore } from 'pinia';
import { ref } from 'vue';

// The note composer of each task's Details tab: whether it is revealed and the text typed so far. In
// memory only, as `adePanelTabs`: hidden on every app start, kept across tab, task and panel
// switches. Entries of tasks that left the board are pruned.
export const useAdeNoteComposerStore = defineStore('adeNoteComposer', () => {
  const opened = ref<Record<string, boolean>>({});
  const drafts = ref<Record<string, string>>({});

  const isOpen = (taskId: string): boolean => opened.value[taskId] ?? false;
  const draftOf = (taskId: string): string => drafts.value[taskId] ?? '';

  function show(taskId: string): void {
    opened.value[taskId] = true;
  }
  function setDraft(taskId: string, text: string): void {
    drafts.value[taskId] = text;
  }
  /** Hides the composer and drops its text. */
  function clear(taskId: string): void {
    delete opened.value[taskId];
    delete drafts.value[taskId];
  }
  function prune(taskIds: ReadonlySet<string>): void {
    for (const rec of [opened.value, drafts.value]) {
      for (const k of Object.keys(rec)) if (!taskIds.has(k)) delete rec[k];
    }
  }

  return { isOpen, draftOf, show, setDraft, clear, prune };
});
