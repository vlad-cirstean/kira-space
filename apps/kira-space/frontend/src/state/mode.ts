import type { AppMode } from '@shared/domain/mode';
import { createModeStore } from '@workbench/state/createModeStore';
import { control } from '../bridge/control';

export const useModeStore = createModeStore<AppMode>(control, 'git', () => ({}));
