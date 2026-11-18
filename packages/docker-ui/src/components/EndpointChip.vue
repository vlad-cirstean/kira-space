<script setup lang="ts">
import { useQuery, useQueryClient } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import PickerTrigger from '@theme/components/PickerTrigger.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
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

async function onSelect(value: unknown): Promise<void> {
  const name = value === AUTOMATIC ? '' : String(value);
  const next = await control.useContext(name);
  qc.setQueryData<DockerStatus>(['docker', 'status'], next);
  await qc.invalidateQueries({ queryKey: ['docker', 'contexts'] });
}
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
      <!-- radio: MenuItem has no radio kind -->
      <DropdownMenuRadioGroup :model-value="selected" @update:model-value="onSelect">
        <DropdownMenuRadioItem
          :value="AUTOMATIC"
          class="h-auto min-h-control py-1"
          data-testid="docker-context-automatic"
        >
          <TruncatedText class="flex-1" :text="t('docker.endpoint.automatic.option')" />
          <template #indicator-icon><CodiconIcon name="check" :size="13" /></template>
        </DropdownMenuRadioItem>
        <DropdownMenuSeparator />
        <DropdownMenuRadioItem
          v-for="c in contexts.data.value ?? []"
          :key="c.name"
          :value="c.name"
          class="h-auto min-h-control py-1"
          data-testid="docker-context-option"
          :data-value="c.name"
        >
          <span class="flex min-w-0 flex-1 flex-col">
            <TruncatedText :text="c.name" />
            <TruncatedText :text="c.host" :class="rowMeta" />
          </span>
          <template #indicator-icon><CodiconIcon name="check" :size="13" /></template>
        </DropdownMenuRadioItem>
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>
  <Tooltip v-if="endpoint && !endpoint.secure">
    <TooltipTrigger as-child>
      <span class="flex items-center text-warn" data-testid="docker-insecure">
        <span class="sr-only">{{ t('docker.endpoint.insecure.tooltip') }}</span>
        <CodiconIcon name="warning" :size="12" />
      </span>
    </TooltipTrigger>
    <TooltipContent>{{ t('docker.endpoint.insecure.tooltip') }}</TooltipContent>
  </Tooltip>
</template>
