<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { kindIcon } from '../kinds';
import { useDescribe } from '../queries';
import { type KubeTab, type KubeTabView, useKubeTabsStore } from '../tabs';
import DescribeView from './DescribeView.vue';
import LogsView from './LogsView.vue';

const props = defineProps<{ tab: KubeTab }>();

const tabs = useKubeTabsStore();
const target = computed(() => props.tab.state.ref);
const describe = useDescribe({
  clusterId: () => target.value.clusterId,
  kind: () => target.value.kind.key,
  namespace: () => target.value.namespace,
  name: () => target.value.name,
});

const items = computed(() => [
  {
    value: 'describe',
    label: t('kubernetes.detail.tab.describe.label'),
    testid: 'kube-detail-tab-describe',
  },
  ...(target.value.kind.logs
    ? [{ value: 'logs', label: t('docker.tab.logs.label'), testid: 'kube-detail-tab-logs' }]
    : []),
]);
const view = computed<KubeTabView>(() =>
  props.tab.state.view === 'logs' && target.value.kind.logs ? 'logs' : 'describe',
);
</script>

<template>
  <section
    class="flex h-full min-h-0 flex-col"
    :aria-label="target.name"
    data-testid="kube-detail"
    :data-tab-id="tab.id"
    :data-active="tab.active"
  >
    <div class="flex h-bar shrink-0 items-center gap-2 px-2">
      <span class="min-w-0 shrink truncate text-muted-foreground">
        {{ target.context }}
        ›
        <template v-if="target.namespace">{{ target.namespace }} ›</template>
      </span>
      <CodiconIcon :name="kindIcon(target.kind)" :size="13" class="shrink-0" />
      <span class="truncate font-medium" data-testid="kube-detail-title">{{ target.name }}</span>
      <SecondaryTabs
        :model-value="view"
        :items="items"
        @update:model-value="(v) => tabs.setView(tab.id, v as KubeTabView)"
      />
    </div>
    <div class="min-h-0 flex-1">
      <LogsView
        v-if="view === 'logs' && describe.data.value"
        :cluster-id="target.clusterId"
        :namespace="target.namespace"
        :pod="target.name"
        :containers="describe.data.value.containers"
      />
      <DescribeView
        v-else-if="view === 'describe'"
        :cluster-id="target.clusterId"
        :kind="target.kind.key"
        :namespace="target.namespace"
        :name="target.name"
      />
    </div>
  </section>
</template>
