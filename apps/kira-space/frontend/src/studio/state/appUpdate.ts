import { createAppUpdateStore } from '@workbench/state/createAppUpdateStore';
import { control } from '../bridge/control';

// P119: hoisted into createAppUpdateStore.ts, shared with Kira Space's own copy of this file
// (createKeepAwakeStore.ts's own per-app-call-site precedent).
export const useAppUpdateStore = createAppUpdateStore(control, 'Kira Studio');
