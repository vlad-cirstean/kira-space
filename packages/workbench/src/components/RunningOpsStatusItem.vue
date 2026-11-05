<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { formatDuration } from '@theme/format';
import { t } from '@theme/i18n';
import { useIntervalFn, useNow } from '@vueuse/core';
import { ref, watch } from 'vue';
import { TIMING } from '../behaviour/timing';
import type { RunningOpItem } from '../state/runningOps';
import StatusBarItem from './StatusBarItem.vue';

// Status bar widget: every running op with a stop action; hidden at zero. The elapsed clock ticks
// only while the popover is open.
defineProps<{ items: readonly RunningOpItem[] }>();
const emit = defineEmits<{ open: [] }>();

const open = ref(false);
let clock: ReturnType<typeof useIntervalFn> | undefined;
const now = useNow({
  scheduler: (cb) => {
    clock = useIntervalFn(cb, TIMING.clockTick, { immediate: false });
    return clock;
  },
});
watch(open, (isOpen) => {
  if (!isOpen) return clock?.pause();
  now.value = new Date();
  clock?.resume();
});

function showOperations(): void {
  open.value = false;
  emit('open');
}
</script>

<template>
  <Popover v-if="items.length > 0" v-model:open="open">
    <PopoverTrigger as-child>
      <StatusBarItem icon="pulse" data-testid="status-ops">
        {{ t('common.state.runningCount.status', { count: items.length }) }}
      </StatusBarItem>
    </PopoverTrigger>
    <PopoverContent side="top" align="end" class="w-96 p-1" data-testid="status-ops-popover">
      <div class="px-2 py-1 text-kira-sm font-medium text-muted-foreground">
        {{ t('workbench.statusBar.ops.title') }}
      </div>
      <div class="flex flex-col gap-0.5">
        <div
          v-for="item in items"
          :key="item.id"
          class="flex items-center gap-2 rounded-kira-sm px-2 py-1"
          data-testid="status-ops-row"
          :data-op-id="item.id"
        >
          <div class="min-w-0 flex-1">
            <div class="truncate text-kira-md">{{ item.label }}</div>
            <div v-if="item.detail" class="truncate text-kira-sm text-muted-foreground">
              {{ item.detail }}
            </div>
          </div>
          <span class="shrink-0 text-kira-sm tabular-nums text-muted-foreground">{{
            formatDuration(Math.max(0, now.getTime() - item.startedAt))
          }}</span>
          <TooltipIconButton
            icon="debug-stop"
            :label="
              item.canStop
                ? t('workbench.statusBar.ops.stop.tooltip', { name: item.label })
                : t('workbench.statusBar.ops.notStoppable.tooltip')
            "
            :disabled="!item.canStop"
            :disabled-trigger="!item.canStop"
            data-testid="status-ops-stop"
            @click="item.stop()"
          />
        </div>
      </div>
      <div class="flex justify-end px-1 pt-1">
        <Button
          variant="link"
          size="inline"
          data-testid="status-ops-open"
          @click="showOperations"
          >{{
            t('workbench.statusBar.ops.open.label')
          }}</Button
        >
      </div>
    </PopoverContent>
  </Popover>
</template>
