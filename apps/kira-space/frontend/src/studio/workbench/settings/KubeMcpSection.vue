<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { FieldDescription, FieldLegend } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed, ref } from 'vue';
import ClusterMcpDialog from '../../kubernetes/components/ClusterMcpDialog.vue';
import { useClusters, useSetClusterMcp } from '../../kubernetes/queries';
import type { KubeCluster } from '../../kubernetes/wire';
import McpExposureRow from './McpExposureRow.vue';

// Which kubeconfig clusters an agent on the Studio MCP server may list and read. The switch flips
// the cluster's `enabled`; description and approval mode are edited in the dialog (Edit).
const clusters = useClusters();
const list = computed(() => clusters.data.value?.clusters ?? []);
const save = useSetClusterMcp();

const editId = ref('');
const editOpen = computed({
  get: () => editId.value !== '',
  set: (v) => {
    if (!v) editId.value = '';
  },
});

const toggle = useAsyncAction((c: KubeCluster, enabled: boolean) =>
  save.mutateAsync({ clusterId: c.id, mcp: { ...c.mcp, enabled } }),
);
</script>

<template>
  <FieldLegend>{{ t('studio.settings.dbMcp.kube.legend.label') }}</FieldLegend>
  <FieldDescription>{{ t('studio.settings.dbMcp.kube.intro.description') }}</FieldDescription>
  <ul
    v-if="list.length"
    class="flex flex-col m-0 p-0 list-none gap-0.5"
    data-testid="db-mcp-kube-list"
  >
    <li v-for="cluster in list" :key="cluster.id">
      <McpExposureRow
        :id="cluster.id"
        testid="db-mcp-kube"
        :name="cluster.context"
        :enabled="cluster.mcp.enabled"
        :status="
          t(
            cluster.mcp.approval === 'allow'
              ? 'studio.settings.dbMcp.kube.allow.status'
              : 'studio.settings.dbMcp.kube.prompt.status',
          )
        "
        :pending="toggle.pending.value"
        :toggle-aria="t('studio.settings.dbMcp.kube.toggle.aria', { name: cluster.context })"
        @toggle="(on) => toggle.run(cluster, on)"
      >
        <template #detail>
          <FieldDescription v-if="cluster.mcp.description">{{
            cluster.mcp.description.split('\n', 1)[0]
          }}</FieldDescription>
        </template>
        <template #actions>
          <Button
            variant="dialog"
            size="kira-lg"
            :data-testid="`db-mcp-kube-edit-${cluster.id}`"
            @click="editId = cluster.id"
          >
            {{ t('common.action.edit.label') }}
          </Button>
        </template>
      </McpExposureRow>
    </li>
  </ul>
  <FieldDescription v-else data-testid="db-mcp-kube-empty">
    {{ t('studio.settings.dbMcp.kube.empty.empty') }}
  </FieldDescription>
  <Alert v-if="toggle.error.value" variant="error" data-testid="db-mcp-kube-error">
    <AlertDescription>{{ toggle.error.value }}</AlertDescription>
  </Alert>
  <ClusterMcpDialog v-if="editId" v-model:open="editOpen" :cluster-id="editId" />
</template>
