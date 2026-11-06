import { useLayoutStore } from '../../state/layout';
import { pinia } from '../../state/pinia';
import { useSettingsStore } from '../../state/settings';
import { control } from '../bridge/control';
import { useProofWindowStore } from './state';

/** A proof window boots only what it shows: settings and theme, the layout (cell editor height),
 *  the session selection, and the window registration. No connections, tabs, terminals, MCP, ops
 *  or schema. */
export async function hydrateProofWindow(): Promise<void> {
  await Promise.all([
    useSettingsStore(pinia).hydrateSettings(),
    useLayoutStore(pinia).hydrateLayout(),
  ]);
  const proofWindow = useProofWindowStore(pinia);
  const { sessionId } = await control.proofWindowTarget();
  proofWindow.selectSession(sessionId);
  proofWindow.active = true;
}
