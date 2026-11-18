<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import Identifier from '@workbench/components/Identifier.vue';
import { computed } from 'vue';
import { useRetryStatus } from '../queries';
import type { DockerStatus, UnavailableReason } from '../wire';
import EndpointChip from './EndpointChip.vue';

const props = defineProps<{ status: DockerStatus }>();

const { retrying, retry } = useRetryStatus();

const COPY: Record<UnavailableReason, { title: () => string; hint: () => string }> = {
  'not-installed': {
    title: () => t('docker.unavailable.notInstalled.title'),
    hint: () =>
      t('docker.unavailable.notInstalled.hint', { engines: 'Docker Desktop, Colima or Podman' }),
  },
  'daemon-down': {
    title: () => t('docker.unavailable.daemonDown.title'),
    hint: () => t('docker.unavailable.daemonDown.hint'),
  },
  'permission-denied': {
    title: () => t('docker.unavailable.permissionDenied.title'),
    hint: () => t('docker.unavailable.permissionDenied.hint'),
  },
  unreachable: {
    title: () => t('docker.unavailable.unreachable.title'),
    hint: () => t('docker.unavailable.unreachable.hint', { env: 'DOCKER_HOST' }),
  },
  tls: {
    title: () => t('docker.unavailable.tls.title'),
    hint: () => t('docker.unavailable.tls.hint', { env: 'DOCKER_CERT_PATH' }),
  },
  error: {
    title: () => t('docker.unavailable.error.title'),
    hint: () => t('docker.unavailable.error.hint'),
  },
};

const copy = computed(() => {
  const c = COPY[props.status.reason ?? 'error'];
  return { title: c.title(), hint: c.hint() };
});
</script>

<template>
  <EmptyState
    fill
    icon="debug-disconnect"
    data-testid="docker-unavailable"
    :title="copy.title"
    :description="copy.hint"
  >
    <Identifier :value="status.endpoint.host" data-testid="docker-unavailable-endpoint" />
    <FieldDescription v-if="status.message" class="max-w-md">{{ status.message }}</FieldDescription>
    <div class="flex items-center gap-2">
      <Button
        size="kira"
        variant="toolbar"
        :pending="retrying"
        data-testid="docker-retry"
        @click="retry"
      >
        <CodiconIcon name="refresh" :size="13" />
        {{ t('common.action.retry.label') }}
      </Button>
      <EndpointChip />
    </div>
    <FieldDescription>{{ t('docker.unavailable.recheck.hint') }}</FieldDescription>
  </EmptyState>
</template>
