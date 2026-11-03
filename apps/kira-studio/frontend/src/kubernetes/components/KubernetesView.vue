<script setup lang="ts">
import { ResizeHandle } from '@theme/components/ui/resizable';
import { useLiveSize } from '@theme/components/ui/resizable/useLiveSize';
import { t } from '@theme/i18n';
import { useElementSize } from '@vueuse/core';
import ModuleStart from '@workbench/components/ModuleStart.vue';
import QueryState from '@workbench/components/QueryState.vue';
import { computed, ref, watch } from 'vue';
import { useClusterStatus, useClusters } from '../queries';
import { useKubernetesUiStore } from '../state';
import { useKubeTabsStore } from '../tabs';
import ClusterUnavailable from './ClusterUnavailable.vue';
import ResourceBrowser from './ResourceBrowser.vue';
import ResourceTabs from './ResourceTabs.vue';

const LIST_MIN = 280;
const TABS_MIN = 360;

const ui = useKubernetesUiStore();
const tabs = useKubeTabsStore();
const clusters = useClusters();
const cluster = computed(() => clusters.data.value?.clusters.find((c) => c.id === ui.clusterId));
const config = computed(() => clusters.data.value?.config);
const missingArgs = (paths: string[]) => ({
  paths: paths.join(', '),
  env: 'KUBECONFIG',
  path: '~/.kube/config',
});
const status = useClusterStatus(() => cluster.value?.id ?? '');

const rowEl = ref<HTMLElement | null>(null);
const { width: rowWidth } = useElementSize(rowEl);
const pane = useLiveSize(
  () => ui.listWidth,
  (w) => {
    ui.listWidth = w;
  },
);
const hasTabs = computed(() => tabs.count > 0);
const listShown = computed(() => !ui.listHidden || !hasTabs.value);

// A context removed from the kubeconfig takes its tabs.
watch(
  () => clusters.data.value,
  (data) => {
    if (!data) return;
    const live = new Set(data.clusters.map((c) => c.id));
    tabs.dropClusters(
      [...new Set(tabs.tabs.map((tab) => tab.state.ref.clusterId))].filter((id) => !live.has(id)),
    );
  },
);
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="kubernetes-view">
    <QueryState :query="clusters" :is-empty="() => false">
      <ModuleStart
        v-if="config?.state === 'missing'"
        icon="type-hierarchy"
        data-testid="kube-config-missing"
        :title="t('kubernetes.config.missing.title')"
        :description="t('kubernetes.config.missing.body', missingArgs(config.paths))"
      />
      <ModuleStart
        v-else-if="config?.state === 'invalid'"
        icon="type-hierarchy"
        data-testid="kube-config-invalid"
        :title="t('kubernetes.config.invalid.title')"
        :description="t('kubernetes.config.invalid.body', { message: config.message })"
      />
      <div v-else ref="rowEl" class="flex h-full min-h-0">
        <section
          v-show="listShown"
          :class="['flex min-h-0 min-w-0 flex-col', hasTabs ? 'flex-none' : 'flex-1']"
          :style="hasTabs ? { width: `${pane.size}px` } : undefined"
          data-testid="kube-resources"
        >
          <QueryState v-if="cluster" :query="status" :is-empty="() => false">
            <template #default="{ data }">
              <ClusterUnavailable
                v-if="data.state === 'unavailable'"
                :cluster-id="cluster.id"
                :status="data"
              />
              <ResourceBrowser v-else :cluster="cluster" />
            </template>
          </QueryState>
          <ModuleStart
            v-else
            icon="type-hierarchy"
            data-testid="kube-start"
            :title="t('kubernetes.start.title')"
            :description="t('kubernetes.start.body')"
          />
        </section>
        <ResizeHandle
          v-if="hasTabs && listShown"
          axis="x"
          line
          :label="t('kubernetes.tabs.resize.label')"
          :min="LIST_MIN"
          :max="Math.max(LIST_MIN, rowWidth - TABS_MIN)"
          v-bind="pane.bind"
        />
        <ResourceTabs v-if="hasTabs" />
      </div>
    </QueryState>
  </div>
</template>
