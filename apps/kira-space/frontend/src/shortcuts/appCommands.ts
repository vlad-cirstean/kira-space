import type { PaletteCommand } from '@workbench/shortcuts/palette';
import { useTabSelectionStore } from '@workbench/state/tabSelection';
import { useModeStore } from '../state/mode';
import { useTabsStore } from '../state/tabs';
import { KUBE_TAB_SCOPE } from '../studio/kubernetes/tabHost';
import { useKubeTabsStore } from '../studio/kubernetes/tabs';
import { studioPaletteCommands } from '../studio/shortcuts/studioCommands';

// Kubernetes mode keeps its own tab store; the shell store holds no tab there.
const kubernetesActive = (): boolean => useModeStore().active === 'kubernetes';

/** Tab stepping and closing for the Window menu. */
export function nextTab(): void {
  if (kubernetesActive()) useKubeTabsStore().activateNext();
  else useTabsStore().activateNextTab();
}

export function prevTab(): void {
  if (kubernetesActive()) useKubeTabsStore().activatePrev();
  else useTabsStore().activatePrevTab();
}

export function closeTab(): void {
  if (!kubernetesActive()) {
    useTabsStore().closeActiveTab();
    return;
  }
  const kube = useKubeTabsStore();
  const selection = useTabSelectionStore();
  // The active tab inside a selection of two or more closes the selection (P727).
  if (kube.activeId !== null && selection.isMulti(KUBE_TAB_SCOPE, kube.activeId)) {
    kube.closeMany(selection.selected(KUBE_TAB_SCOPE));
  } else kube.closeActive();
}

/** The palette holds only commands with no other entry point (no button, menu item or chord);
 *  today the three API ones, which only API mode registers. Call once from App setup. */
export function appPaletteCommands(isMainWindow: () => boolean): () => PaletteCommand[] {
  const modeStore = useModeStore();
  return () => (isMainWindow() && modeStore.active === 'api' ? studioPaletteCommands() : []);
}
