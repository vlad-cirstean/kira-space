import { createKeepAwakeStore } from '@workbench/state/createKeepAwakeStore';
import { control } from '../bridge/control';

// P116 H6: the whole store is the shared one (hydrate, subscribe, the manual toggle). P188: the
// agent-aware Settings leaf is Kira Space only, so nothing extends it here.
export const useKeepAwakeStore = createKeepAwakeStore(control, () => ({}));
