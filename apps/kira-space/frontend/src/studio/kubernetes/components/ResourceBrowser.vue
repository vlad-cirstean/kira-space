<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { toCodedError } from '@workbench/bridge/codedError';
import QueryState from '@workbench/components/QueryState.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, ref, watch } from 'vue';
import { defaultKind } from '../kinds';
import { useKinds, useNamespaces, useResourceList } from '../queries';
import { useKubernetesUiStore } from '../state';
import type { KubeCluster } from '../wire';
import KindBar from './KindBar.vue';
import NamespacePicker from './NamespacePicker.vue';
import ResourceTable from './ResourceTable.vue';

const props = defineProps<{ cluster: KubeCluster }>();

const ui = useKubernetesUiStore();
const kinds = useKinds(() => props.cluster.id);
// The stored pick wins while listed; nothing is written until the user picks.
const kind = computed(() => {
  const all = kinds.data.value?.kinds ?? [];
  return all.find((k) => k.key === ui.kind) ?? defaultKind(all);
});
const namespaced = computed(() => kind.value?.namespaced === true);
const namespaces = useNamespaces(
  () => props.cluster.id,
  () => namespaced.value,
);

// Unset: the context's namespace, else all. A restricted account falls back to the namespace the
// server allows.
const namespace = computed(() => {
  if (!namespaced.value) return '';
  if (ui.namespace !== null) return ui.namespace;
  if (namespaces.data.value?.restricted) return namespaces.data.value.fallback;
  return props.cluster.namespace;
});

const list = useResourceList({
  clusterId: () => props.cluster.id,
  kind: () => kind.value?.key ?? '',
  namespace,
  labelSelector: () => ui.labelSelector,
  enabled: () => kind.value !== undefined && (!namespaced.value || !namespaces.isPending.value),
});

const selector = ref(ui.labelSelector);
watch(
  () => ui.labelSelector,
  (v) => {
    selector.value = v;
  },
);
function applySelector(): void {
  ui.labelSelector = selector.value.trim();
}

const badSelector = computed(() => {
  const err = list.error.value;
  return err && toCodedError(err).code === 'E_BAD_REQUEST' ? toCodedError(err).message : '';
});

const shownCount = computed(() => list.data.value?.rows.length ?? 0);
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="kube-browser">
    <QueryState :query="kinds" :is-empty="() => (kinds.data.value?.kinds.length ?? 0) === 0">
      <template #empty>
        <EmptyState fill data-testid="kube-kinds-empty" :title="t('kubernetes.kinds.none.empty')" />
      </template>
      <template #default="{ data }">
        <ViewToolbar>
          <KindBar
            :kinds="data.kinds"
            :value="kind?.key ?? ''"
            :recent="ui.recent"
            :partial="data.partial"
            @select="ui.selectKind"
          />
          <span class="ml-auto text-kira-sm text-muted-foreground" data-testid="kube-count">{{
            t('kubernetes.list.count.status', { count: shownCount })
          }}</span>
          <TooltipIconButton
            icon="refresh"
            :label="t('common.action.refresh.label')"
            data-testid="kube-list-refresh"
            @click="list.refetch()"
          />
        </ViewToolbar>
        <ViewToolbar class="flex-wrap">
          <NamespacePicker
            v-if="namespaced"
            :value="namespace"
            :data="namespaces.data.value"
            @change="ui.setNamespace($event)"
          />
          <SearchField
            v-model="ui.listFilter"
            class="min-w-32 flex-1"
            :placeholder="t('kubernetes.list.filter.placeholder')"
            data-testid="kube-list-filter"
          />
          <Input
            v-model="selector"
            class="w-40"
            :placeholder="t('kubernetes.list.labelSelector.placeholder')"
            :aria-label="t('kubernetes.list.labelSelector.label')"
            :aria-invalid="badSelector !== ''"
            data-testid="kube-label-selector"
            @change="applySelector"
          />
        </ViewToolbar>
        <FieldError v-if="badSelector" class="px-2" data-testid="kube-selector-error">{{
          badSelector
        }}</FieldError>
        <div class="flex min-h-0 flex-1 flex-col">
          <QueryState v-if="!badSelector && kind" :query="list" :is-empty="() => false">
            <template #default="{ data: rows }">
              <ResourceTable
                :data="rows"
                :cluster-id="cluster.id"
                :context="cluster.context"
                :kind="kind"
              />
            </template>
          </QueryState>
        </div>
      </template>
    </QueryState>
  </div>
</template>
