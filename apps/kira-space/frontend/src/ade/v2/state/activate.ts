import type { App } from 'vue';
import { useRepoLinksStore } from '../../../repo/state/repoLinks';
import { useAdeBoardUiStore } from './adeBoardUi';
import { useAdeDetailsListStore } from './adeDetailsList';
import { useAdeDialogsStore } from './adeDialogs';
import { useAdeLeasesStore } from './adeLeases';
import { useAdeOpsStore } from './adeOps';
import { useAdeRepoGateStore } from './adeRepoGate';
import { useAdeStageDialogStore } from './adeStageDialog';
import { useAdeTakeOverStore } from './adeTakeOver';
import { useAdeTerminalsStore } from './adeTerminals';

let activated = false;

// A watcher created while a component is the current instance pins that component, and through it
// the whole mounted tree, after unmount: the store scope outlives the component. Creating each store
// that owns a watcher, query or mutation here, with no current instance, keeps the ADE tree
// collectable. A new store of that kind joins this list.
export function activateAdeStores(app: App): void {
  if (activated) return;
  activated = true;
  app.runWithContext(() => {
    useRepoLinksStore();
    useAdeBoardUiStore();
    useAdeDetailsListStore();
    useAdeOpsStore();
    useAdeDialogsStore();
    useAdeStageDialogStore();
    useAdeLeasesStore();
    useAdeTakeOverStore();
    useAdeRepoGateStore();
    useAdeTerminalsStore();
  });
}
