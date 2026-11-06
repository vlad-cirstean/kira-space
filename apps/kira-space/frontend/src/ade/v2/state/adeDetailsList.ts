import { usePersistedState } from '@workbench/behaviour/persisted';
import { defineStore } from 'pinia';

// How the Details list renders. One preference for all tasks, kept per device.
export const useAdeDetailsListStore = defineStore('adeDetailsList', () => {
  /** Whether task events (creation, branches, stored artifacts) join the notes and proof. */
  const showTimeline = usePersistedState('kira.ade.details.timeline', false);

  function setShowTimeline(on: boolean): void {
    showTimeline.value = on;
  }

  return { showTimeline, setShowTimeline };
});
