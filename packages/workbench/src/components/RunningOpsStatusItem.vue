<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import {
  Popover,
  PopoverContent,
  PopoverFooter,
  PopoverList,
  PopoverTrigger,
} from '@theme/components/ui/popover';
import { formatDuration } from '@theme/format';
import { t } from '@theme/i18n';
import { rowIndent, rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
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
    <PopoverContent side="top" align="end" flush class="w-96" data-testid="status-ops-popover">
      <SectionHeading :label="t('workbench.statusBar.ops.title')" />
      <PopoverList>
        <div
          v-for="item in items"
          :key="item.id"
          :class="cn(rowVariants({ layout: 'double' }), 'items-center')"
          :style="rowIndent(0)"
          data-testid="status-ops-row"
          :data-op-id="item.id"
        >
          <div class="min-w-0 flex-1">
            <div class="truncate text-kira-md">{{ item.label }}</div>
            <div v-if="item.detail" :class="cn('truncate', rowMeta)">{{ item.detail }}</div>
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
      </PopoverList>
      <PopoverFooter>
        <Button variant="toolbar" size="kira" data-testid="status-ops-open" @click="showOperations">
          <CodiconIcon name="link-external" :size="13" />
          {{ t('workbench.statusBar.ops.open.label') }}
        </Button>
      </PopoverFooter>
    </PopoverContent>
  </Popover>
</template>
