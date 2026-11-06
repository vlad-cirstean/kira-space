import type { AppMode } from '@shared/domain/mode';
import { t } from '@theme/i18n';
import type { ModeRegistry } from '@workbench/modes';
import { defineAsyncComponent } from 'vue';
import AdeView from '../ade/AdeView.vue';
import GitNewTab from '../repo/GitNewTab.vue';
import GitPanel from '../repo/GitPanel.vue';
import GitStart from '../repo/GitStart.vue';
import ApiStart from '../studio/api/ApiStart.vue';
import CollectionsPanel from '../studio/api/CollectionsPanel.vue';
import ProjectPanel from '../studio/workbench/panels/ProjectPanel.vue';
import StudioStart from '../studio/workbench/panels/StudioStart.vue';

// Space modules first, then the database, API, Docker and Kubernetes modules.
/** Modes whose project panel lists connections (database, API, Docker, Kubernetes). */
export const CONNECTION_PANEL_MODES: ReadonlySet<AppMode> = new Set([
  'studio',
  'api',
  'docker',
  'kubernetes',
]);

export const MODE_ORDER: AppMode[] = [
  'ade',
  'automations',
  'memory',
  'git',
  'studio',
  'api',
  'docker',
  'kubernetes',
];

export const MODES: ModeRegistry<AppMode> = {
  git: {
    get label() {
      return t('common.module.git.label');
    },
    icon: 'source-control',
    panel: GitPanel,
    start: GitStart,
    newTab: GitNewTab,
  },
  // A full-area view: no left panel, no tab strip, no tabs.
  ade: {
    get label() {
      return t('mobile.shell.heading.title');
    },
    icon: 'robot',
    layout: 'full',
    view: AdeView,
  },
  automations: {
    get label() {
      return t('common.module.automations.label');
    },
    icon: 'run-all',
    panel: defineAsyncComponent(() => import('@workbench/automations/AutomationsPanel.vue')),
    start: defineAsyncComponent(() => import('@workbench/automations/AutomationsStart.vue')),
    newTab: defineAsyncComponent(() => import('@workbench/automations/AutomationsNewTab.vue')),
  },
  // A panel module that opens no tabs; the detail view is its main area.
  memory: {
    get label() {
      return t('common.module.memory.label');
    },
    icon: 'lightbulb',
    panel: defineAsyncComponent(() => import('@workbench/memory/MemoryPanel.vue')),
    start: defineAsyncComponent(() => import('@workbench/memory/MemoryStart.vue')),
    tabStrip: false,
  },
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
  docker: {
    get label() {
      return t('common.product.docker.label');
    },
    icon: 'vm',
    tabStrip: false,
    panel: defineAsyncComponent(() => import('../studio/docker/DockerPanel.vue')),
    start: defineAsyncComponent(() => import('../studio/docker/DockerView.vue')),
  },
  kubernetes: {
    get label() {
      return t('common.product.kubernetes.label');
    },
    icon: 'type-hierarchy',
    tabStrip: false,
    panel: defineAsyncComponent(
      () => import('../studio/kubernetes/components/KubernetesPanel.vue'),
    ),
    start: defineAsyncComponent(() => import('../studio/kubernetes/components/KubernetesView.vue')),
  },
};
