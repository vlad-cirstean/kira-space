import { usePersistedState } from '@workbench/behaviour/persisted';
import { defineStore } from 'pinia';

// How the Details list renders. One preference for all tasks, kept per device.
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

  return { showTimeline, setShowTimeline, showDbAudit, setShowDbAudit };
});
