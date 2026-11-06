import { onScopeDispose, reactive, watchEffect } from 'vue';

// Views publish "this tab holds an unsaved edit buffer" so the tab kind's `isDirty` can read it
// without importing the view.
const dirtyTabs = reactive(new Set<string>());

export function usePublishEditDirty(tabId: () => string, dirty: () => boolean): void {
  let published: string | null = null;
  function clear(): void {
    if (published !== null) dirtyTabs.delete(published);
    published = null;
  }
  watchEffect(() => {
    const id = tabId();
    const isDirty = dirty();
    clear();
    if (isDirty) {
      dirtyTabs.add(id);
      published = id;
    }
  });
  onScopeDispose(clear);
}

export function isEditDirty(tabId: string): boolean {
  return dirtyTabs.has(tabId);
}
