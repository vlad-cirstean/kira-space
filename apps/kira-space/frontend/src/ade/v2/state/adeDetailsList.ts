import { usePersistedState } from '@workbench/behaviour/persisted';
import { defineStore } from 'pinia';
import { reactive } from 'vue';

// How the Details list renders. Preferences are kept per device; which long entries are expanded
// stays in memory only.
export const useAdeDetailsListStore = defineStore('adeDetailsList', () => {
  /** Whether task events (creation, branches, stored artifacts) join the notes and proof. */
  const showTimeline = usePersistedState('kira.ade.details.timeline', false);

  /** Whether the Kira Studio audit trail joins the list. On by default: every action is recorded. */
  const showDbAudit = usePersistedState('kira.ade.details.dbAudit', true);

  function setShowDbAudit(on: boolean): void {
    showDbAudit.value = on;
  }

  function setShowTimeline(on: boolean): void {
    showTimeline.value = on;
  }

  // Survives the Details tab remounting on a tab switch, so kept scroll lands on the same layout.
  const expanded = reactive(new Set<string>());

  function isExpanded(key: string): boolean {
    return expanded.has(key);
  }

  function setExpanded(key: string, on: boolean): void {
    if (on) expanded.add(key);
    else expanded.delete(key);
  }

  return {
    showTimeline,
    setShowTimeline,
    showDbAudit,
    setShowDbAudit,
    expanded,
    isExpanded,
    setExpanded,
  };
});
