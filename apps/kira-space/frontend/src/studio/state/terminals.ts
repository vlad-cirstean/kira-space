import { createTerminalsStore } from '@workbench/state/createTerminalsStore';
import { control } from '../bridge/control';
import { useSettingsStore } from './settings';

// The terminal registry — session state by tab id, plus output routing. Lives in
// `state/`, not `terminal/` or `views/`: biome.json forbids `views/**` importing `workbench/**`,
// so `state/` is the one layer AutomationsPanel.vue (the indicator), TabStrip.vue (the "+"),
// state/tabKinds.ts (dropResources) and views/terminal/ (the renderer) can all reach.
//
// The shared skeleton (byte-identical to Kira Space's own state/terminals.ts)
// now lives in packages/workbench/src/state/createTerminalsStore.ts. This file is the
// `control` wiring.

export const useTerminalsStore = createTerminalsStore(control, {
  appearance: () => useSettingsStore().appearance,
  openExternal: control.openExternal,
});
