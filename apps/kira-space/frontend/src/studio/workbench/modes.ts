import type { AppMode } from '@shared/domain/mode';
import { t } from '@theme/i18n';
import type { ModeRegistry, PanelModeDef } from '@workbench/modes';
import { defineAsyncComponent } from 'vue';
import ApiStart from '../api/ApiStart.vue';
import CollectionsPanel from '../api/CollectionsPanel.vue';
import ProjectPanel from './panels/ProjectPanel.vue';
import StudioStart from './panels/StudioStart.vue';

// P91 OQ-1: Terminal joins Studio/Api last, the plan's own stated default. Hoisted here from
// the old TitleBar.vue at P128 §2.3: one file lists a module.
export const MODE_ORDER: AppMode[] = ['studio', 'api', 'automations', 'docker', 'kubernetes'];

// Mode content comes from a registry, mirroring the tab-kind registry. Api's own entries are both
// EmptyState-based — this adds no HTTP functionality, only the seam. `Component` already covers
// `defineAsyncComponent`'s return either way. `terminal`'s panel/start/newTab now live in the
// shared terminal module (packages/workbench/src/terminal/) — no `modeStore.active === 'terminal'`
// branch anywhere in WorkbenchShell.vue any more, the module owns its own "+" via `newTab`.
// Every one of this app's own modes is a panel module, so `MODES` is typed
// `ModeRegistry<AppMode, PanelModeDef>` rather than the base `ModeDef` union — a one-line type
// change, no behavior change, since this app's own shell already reads `.panel`/`.start`
// unconditionally.
export const MODES: ModeRegistry<AppMode, PanelModeDef> = {
  studio: {
    get label() {
      return t('studio.mode.studio.label');
    },
    icon: 'database',
    panel: ProjectPanel,
    start: StudioStart,
  },
  api: {
    get label() {
      return t('studio.mode.api.label');
    },
    icon: 'globe',
    panel: CollectionsPanel,
    start: ApiStart,
  },
  // P91 §2: a peer module, lazy the same reason Studio/Api's entries are — nothing
  // in a Studio-only session should pay for the terminal panel's own launch chunk.
  automations: {
    get label() {
      return t('common.module.automations.label');
    },
    icon: 'run-all',
    panel: defineAsyncComponent(() => import('@workbench/automations/AutomationsPanel.vue')),
    start: defineAsyncComponent(() => import('@workbench/automations/AutomationsStart.vue')),
    newTab: defineAsyncComponent(() => import('@workbench/automations/AutomationsNewTab.vue')),
  },
  // P200: lazy like Terminal; no tabs of its own, so the tab strip is hidden for this mode.
  docker: {
    get label() {
      return t('common.product.docker.label');
    },
    icon: 'vm',
    tabStrip: false,
    panel: defineAsyncComponent(() => import('../docker/DockerPanel.vue')),
    start: defineAsyncComponent(() => import('../docker/DockerView.vue')),
  },
  // P439: lazy like Docker; no tabs of its own.
  kubernetes: {
    get label() {
      return t('common.product.kubernetes.label');
    },
    icon: 'type-hierarchy',
    tabStrip: false,
    panel: defineAsyncComponent(() => import('../kubernetes/components/KubernetesPanel.vue')),
    start: defineAsyncComponent(() => import('../kubernetes/components/KubernetesView.vue')),
  },
};
