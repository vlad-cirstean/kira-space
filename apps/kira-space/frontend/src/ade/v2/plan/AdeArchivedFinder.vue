<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from '@theme/components/ui/command';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { formatDateTime } from '@workbench/behaviour/format';
import { ref } from 'vue';
import { usePlanModel } from './usePlanModel';

// Search over every archived task, loaded or not. Picking one only reports it; the parent selects
// it, so the archived panel opens beside the Plan.
const emit = defineEmits<{ pick: [taskId: string] }>();
const { history, repoLabel } = usePlanModel();
const open = ref(false);

function pick(taskId: string): void {
  open.value = false;
  emit('pick', taskId);
}
</script>

<template>
  <Popover v-if="history.length" v-model:open="open">
    <PopoverTrigger as-child>
      <TooltipIconButton
        icon="search"
        :label="t('space.ade.plan.history.find.label')"
        data-testid="ade-archived-find"
      />
    </PopoverTrigger>
    <PopoverContent class="w-96 p-0" align="start">
      <Command @update:model-value="(v) => pick(String(v))">
        <CommandInput :placeholder="t('space.ade.plan.history.find.label')" />
        <CommandList>
          <CommandEmpty data-testid="ade-archived-find-empty">{{
            t('common.state.noMatches.empty')
          }}</CommandEmpty>
          <CommandItem
            v-for="e in history"
            :key="e.taskId"
            :value="e.taskId"
            class="flex-col items-start gap-0.5"
            data-testid="ade-archived-find-item"
            :data-task-id="e.taskId"
          >
            <span class="w-full truncate">{{ e.title }}</span>
            <span class="w-full truncate text-kira-sm text-muted-foreground">{{
              [
                e.codeRepoIds.map(repoLabel).join(' · '),
                formatDateTime(e.archivedAt),
                e.mergedAt ? t('common.merged.status') : '',
              ]
                .filter(Boolean)
                .join(' · ')
            }}</span>
          </CommandItem>
        </CommandList>
      </Command>
    </PopoverContent>
  </Popover>
</template>
