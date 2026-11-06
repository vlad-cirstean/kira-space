import { createCustomScriptsStore } from '@workbench/automations/createCustomScriptsStore';
import { control } from '../bridge/control';

export const useCustomScriptsStore = createCustomScriptsStore(control);
