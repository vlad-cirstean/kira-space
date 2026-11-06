import { createLayoutStore } from '@workbench/state/createLayoutStore';
import { control } from '../bridge/control';

export const useLayoutStore = createLayoutStore(control, ({ patchLayout }) => ({
  setCellEditorHeight(height: number): void {
    patchLayout({ panel: { cellEditor: { height } } });
  },
}));
