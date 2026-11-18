<script setup lang="ts">
import { useQuery, useQueryClient } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import PickerTrigger from '@theme/components/PickerTrigger.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import MenuItems from '@workbench/components/MenuItems.vue';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed } from 'vue';
import { useDocker } from '../context';
import { useDockerStatus } from '../queries';
import type { DockerStatus } from '../wire';

const AUTOMATIC = '__automatic__';

const { control } = useDocker();
const qc = useQueryClient();
const status = useDockerStatus();
const contexts = useQuery({
  queryKey: ['docker', 'contexts'],
  queryFn: () => control.contexts(),
  staleTime: 0,
});

const endpoint = computed(() => status.data.value?.endpoint);
const ok = computed(() => status.data.value?.state === 'ok');
const selected = computed(() =>
  endpoint.value?.source === 'selected' ? endpoint.value.context : AUTOMATIC,
);

async function useContext(name: string): Promise<void> {
  const next = await control.useContext(name);
  qc.setQueryData<DockerStatus>(['docker', 'status'], next);
  await qc.invalidateQueries({ queryKey: ['docker', 'contexts'] });
}

const contextItems = computed<MenuItem[]>(() => [
  {
    type: 'radio',
    id: 'docker-context',
    value: selected.value,
    items: [
      {
        type: 'option',
        id: 'docker-context-automatic',
        testId: 'docker-context-automatic',
        value: AUTOMATIC,
        label: t('docker.endpoint.automatic.option'),
        run: () => useContext(''),
      },
      { type: 'separator' },
      ...(contexts.data.value ?? []).map((c) => ({
        type: 'option' as const,
        id: `docker-context-${c.name}`,
        testId: 'docker-context-option',
        value: c.name,
        label: c.name,
        detail: c.host,
        run: () => useContext(c.name),
      })),
    ],
  },
]);
</script>

<template>
  <DropdownMenu :modal="false">
    <DropdownMenuTrigger as-child>
      <PickerTrigger
        size="kira"
        class="max-w-48"
        data-testid="docker-endpoint-chip"
        :aria-label="t('docker.endpoint.chip.aria', { context: endpoint?.context ?? '' })"
        :label="endpoint?.context ?? 'docker'"
      >
        <template #leading>
          <StatusDot
            :tone="ok ? 'ok' : 'error'"
            data-testid="docker-status-dot"
            :data-state="ok ? 'ok' : 'unavailable'"
          />
        </template>
      </PickerTrigger>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="start" data-testid="docker-context-menu">
      <MenuItems :items="contextItems" />
    </DropdownMenuContent>
  </DropdownMenu>
  <TextTooltip v-if="endpoint && !endpoint.secure" :text="t('docker.endpoint.insecure.tooltip')"
    ><span class="flex items-center text-warn" data-testid="docker-insecure">
      <span class="sr-only">{{ t('docker.endpoint.insecure.tooltip') }}</span>
      <CodiconIcon name="warning" :size="12" />
    </span></TextTooltip
  >
</template>
