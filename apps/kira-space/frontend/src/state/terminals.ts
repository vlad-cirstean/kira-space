import { createTerminalsStore } from '@workbench/state/createTerminalsStore';
import { control } from '../bridge/control';
import { useSettingsStore } from './settings';
import { createTerminalFileLinks } from './terminalFileLinks';

// The terminal registry — session state by tab id, plus output routing. Lives in
// `state/`, not `repo/state/`: biome.json forbids `repo/**` importing `views/**` and `views/**`
// importing `workbench/**`, so `state/` is the one layer GitPanel.vue (the indicator),
// TabStrip.vue (the "+"), state/tabKinds.ts (dropResources) and views/repo/ (the renderer) can
// all reach.
//
// The shared skeleton (byte-identical to Kira Studio's own state/terminals.ts)
// now lives in packages/workbench/src/state/createTerminalsStore.ts. This file is the
// `control` wiring.

// The fallback cwd is read lazily, so this module needs no store at init.
export const terminalFileLinks = createTerminalFileLinks(
  (id) => useTerminalsStore().terminalSession(id)?.cwd ?? '',
);

export const useTerminalsStore = createTerminalsStore(control, {
  appearance: () => useSettingsStore().appearance,
  openExternal: control.openExternal,
  fileLinks: terminalFileLinks,
});
