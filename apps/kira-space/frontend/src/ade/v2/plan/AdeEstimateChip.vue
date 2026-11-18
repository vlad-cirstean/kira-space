<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';
import AdeEstimateField from '../panel/AdeEstimateField.vue';
import { useEstimateWrite } from '../panel/useEstimateWrite';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import type { CardModel } from './usePlanModel';

// The estimate on a task row: its text, or a hover-revealed clock when empty. Click opens the
// estimate field (the task menu's Estimate opens it too). The row root is `group/head`.
const props = defineProps<{ card: CardModel }>();

const ui = useAdeBoardUiStore();
const est = useEstimateWrite(() => props.card.task.id);
const local = ref(false);
const open = computed({
  get: () =>
    local.value || (ui.rowEdit?.taskId === props.card.task.id && ui.rowEdit.field === 'estimate'),
  set: (v) => {
    if (v) {
      local.value = true;
      return;
    }
    local.value = false;
    if (ui.rowEdit?.taskId === props.card.task.id) ui.rowEdit = null;
  },
});
</script>

<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <Badge v-if="card.task.est" as-child variant="chip" class="shrink-0 text-muted-foreground">
        <button
          type="button"
          :aria-label="t('space.ade.plan.card.estimate.aria', { value: card.task.est })"
          data-testid="ade-card-est"
          @click.stop
        >
          {{ card.task.est }}
        </button>
      </Badge>
      <Button
        v-else
        type="button"
        variant="toolbar"
        size="inline"
        class="invisible size-4.5 group-hover/head:visible group-focus-visible/head:visible group-has-[:focus-visible]/head:visible data-[state=open]:visible"
        :aria-label="t('space.ade.estimate.field.label')"
        data-testid="ade-card-est"
        @click.stop
      >
        <CodiconIcon name="watch" :size="13" />
      </Button>
    </PopoverTrigger>
    <PopoverContent align="start" class="w-auto" @click.stop>
      <AdeEstimateField
        :est="card.task.est"
        :days="card.entry.span"
        :error="est.error.value"
        :id-prefix="`ade-row-est-${card.task.id}`"
        @save="est.save"
      />
    </PopoverContent>
  </Popover>
</template>
