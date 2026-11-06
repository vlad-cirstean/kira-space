import { createLayoutStore } from '@workbench/state/createLayoutStore';
import { control } from '../bridge/control';

// The shared skeleton now lives in packages/workbench/src/state/createLayoutStore.ts.
// This app's only remaining extra is cell editor height, which Kira Space's UI has no cell editor
// to expose.
export const useLayoutStore = createLayoutStore(control, ({ patchLayout }) => ({
  setCellEditorHeight(height: number): void {
    patchLayout({ panel: { cellEditor: { height } } });
  },
}));
