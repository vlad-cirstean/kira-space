import type { PaletteCommand } from '@workbench/shortcuts/palette';
import { useModeStore } from '../state/mode';
import { useTabsStore } from '../state/tabs';
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
  if (kubernetesActive()) useKubeTabsStore().closeActive();
  else useTabsStore().closeActiveTab();
}

/** The palette holds only commands with no other entry point (no button, menu item or chord);
 *  today the three API ones, which only API mode registers. Call once from App setup. */
export function appPaletteCommands(isMainWindow: () => boolean): () => PaletteCommand[] {
  const modeStore = useModeStore();
  return () => (isMainWindow() && modeStore.active === 'api' ? studioPaletteCommands() : []);
}
