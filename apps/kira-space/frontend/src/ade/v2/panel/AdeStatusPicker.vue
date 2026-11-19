<script setup lang="ts">
import PickerTrigger from '@theme/components/PickerTrigger.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useMutationAction } from '@workbench/behaviour/asyncAction';
import { computed } from 'vue';
import AdeChip from '../AdeChip.vue';
import { taskPatch } from '../board/panelFacts';
import type { CardModel } from '../plan/usePlanModel';
import { usePlanModel } from '../plan/usePlanModel';
import { useUpdateTask } from '../queries';

// Moves the task to any status of the set, in either direction. A manual move holds until the next step change.
const props = defineProps<{ card: CardModel }>();
const { model } = usePlanModel();
const update = useUpdateTask();
const updating = useMutationAction(update);
const error = updating.error;

const statuses = computed(() => model.value?.board.statuses ?? []);
const at = computed(() => statuses.value.findIndex((s) => s.id === props.card.status.id));
const prev = computed(() => (at.value > 0 ? statuses.value[at.value - 1] : undefined));
const next = computed(() =>
  at.value >= 0 && at.value < statuses.value.length - 1 ? statuses.value[at.value + 1] : undefined,
);

async function move(statusId: string): Promise<void> {
  await updating.run({ taskId: props.card.task.id, patch: taskPatch({ statusId }) });
}
</script>

<template>
  <div class="flex flex-col gap-1" data-testid="ade-status-picker">
    <div class="flex items-center gap-1">
      <TooltipIconButton
        :icon="ICON.back"
        :label="t('space.ade.statusPicker.back.tooltip')"
        :disabled="!prev"
        disabled-trigger
        data-testid="ade-status-back"
        @click="move(prev?.id ?? '')"
      />
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <PickerTrigger size="kira" :label="card.status.name" data-testid="ade-status-pick" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" data-testid="ade-status-menu">
          <!-- radio: option body is an AdeChip -->
          <DropdownMenuRadioGroup :model-value="card.status.id">
            <DropdownMenuRadioItem
              v-for="s in statuses"
              :key="s.id"
              :value="s.id"
              :data-testid="`ade-status-option-${s.id}`"
              @select="move(s.id)"
            >
              <AdeChip :label="s.name" :color="s.color" />
            </DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      <TooltipIconButton
        icon="arrow-right"
        :label="t('space.ade.statusPicker.next.tooltip')"
        :disabled="!next"
        disabled-trigger
        data-testid="ade-status-next"
        @click="move(next?.id ?? '')"
      />
    </div>
    <Alert v-if="error" variant="error" data-testid="ade-status-error"
      ><AlertDescription>{{ error }}</AlertDescription></Alert
    >
  </div>
</template>
