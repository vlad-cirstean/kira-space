import { usePersistedState } from '@workbench/behaviour/persisted';
import { defineStore } from 'pinia';

// Which of GitPanel.vue's top-level tabs is showing. Flat, not per-repo: one persistent panel
// across every repo workspace. Own store so hostHandlers.ts's review.open can flip the tab
// without depending on the panel. Persisted (`kira.git.panelTab`), default Repos; a stored value
// outside the two tabs (the removed 'files') reads as Repos.
type RepoPanelTab = 'repos' | 'review';
const PANEL_TABS: readonly string[] = ['repos', 'review'];

export const useRepoPanelTabStore = defineStore('repoPanelTab', () => {
  const panelTab = usePersistedState<string>('kira.git.panelTab', 'repos');

  function repoPanelTab(): RepoPanelTab {
    return PANEL_TABS.includes(panelTab.value) ? (panelTab.value as RepoPanelTab) : 'repos';
  }

  function setRepoPanelTab(tab: RepoPanelTab): void {
    panelTab.value = tab;
  }

  return { repoPanelTab, setRepoPanelTab };
});
