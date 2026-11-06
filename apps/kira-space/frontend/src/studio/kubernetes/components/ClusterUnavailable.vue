<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useRefreshCluster } from '../queries';
import type { KubeStatus, UnavailableReason } from '../wire';

const props = defineProps<{ clusterId: string; status: KubeStatus }>();

const refresh = useRefreshCluster();

const reason = computed<UnavailableReason>(
  () => (props.status.reason || 'unreachable') as UnavailableReason,
);
const COPY: Record<UnavailableReason, { title: () => string; hint: () => string }> = {
  unreachable: {
    title: () => t('kubernetes.unavailable.unreachable.title'),
    hint: () => t('kubernetes.unavailable.unreachable.hint'),
  },
  unauthorized: {
    title: () => t('kubernetes.unavailable.unauthorized.title'),
    hint: () => t('kubernetes.unavailable.unauthorized.hint'),
  },
  forbidden: {
    title: () => t('kubernetes.unavailable.forbidden.title'),
    hint: () => t('kubernetes.unavailable.forbidden.hint'),
  },
  plugin: {
    title: () => t('kubernetes.unavailable.plugin.title'),
    hint: () => t('kubernetes.unavailable.plugin.hint'),
  },
  config: {
    title: () => t('kubernetes.unavailable.config.title'),
    hint: () => t('kubernetes.unavailable.config.hint'),
  },
  timeout: {
    title: () => t('kubernetes.unavailable.timeout.title'),
    hint: () => t('kubernetes.unavailable.timeout.hint'),
  },
};
const title = computed(() => COPY[reason.value].title());
const hint = computed(() => COPY[reason.value].hint());
</script>

<template>
  <EmptyState
    fill
    data-testid="kube-unavailable"
    :data-reason="reason"
    :title="title"
    :description="hint"
  >
    <template #media
      ><CodiconIcon name="debug-disconnect" :size="24" class="text-error" /></template
    >
    <FieldDescription
      v-if="status.message"
      class="max-w-md"
      data-testid="kube-unavailable-message"
      >{{
        status.message
      }}</FieldDescription
    >
    <Button
      size="kira"
      variant="toolbar"
      :pending="refresh.pending.value"
      data-testid="kube-retry"
      @click="refresh.run(clusterId)"
    >
      <CodiconIcon name="refresh" :size="12" />
      {{ t('common.action.retry.label') }}
    </Button>
  </EmptyState>
</template>
