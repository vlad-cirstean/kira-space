import { VueQueryPlugin } from '@tanstack/vue-query';
import { i18n } from '@theme/i18n';
import { provideShellServices } from '@workbench/bootstrapShell';
import { queryClient } from '@workbench/state/queryClient';
import { createApp } from 'vue';
import { control } from '../bridge/control';
import { useLayoutStore } from '../state/layout';
import { pinia } from '../state/pinia';
import { useSettingsStore } from '../state/settings';
import ProofApp from './ProofApp.vue';
import { useProofWindowStore } from './state';

/** A proof window boots only what it shows: settings and theme, the layout (cell editor height),
 *  the session selection, and the window registration. No connections, tabs, terminals, MCP, ops
 *  or schema. */
export async function mountProofWindow(): Promise<void> {
  await control.windowsEnsure();
  await useSettingsStore(pinia).hydrateSettings();
  await useLayoutStore(pinia).hydrateLayout();
  const { sessionId } = await control.proofWindowTarget();
  useProofWindowStore(pinia).selectSession(sessionId);
  const app = createApp(ProofApp);
  app.use(i18n);
  app.use(pinia);
  provideShellServices(app);
  app.use(VueQueryPlugin, { queryClient });
  app.mount('#app');
}
