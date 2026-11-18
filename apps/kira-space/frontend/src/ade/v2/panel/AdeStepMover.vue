<script setup lang="ts">
import PickerTrigger from '@theme/components/PickerTrigger.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import type { CardModel } from '../plan/usePlanModel';
import { useStepMoves } from './useStepMoves';

// Moves the task to any step of its workflow, or to Done, in either direction: each move opens the
// stage dialog, where the handover notes go.
const props = defineProps<{ card: CardModel }>();
const { options, at, prev, next, live, reason, move } = useStepMoves(() => props.card);
</script>

<template>
  <Field v-if="card.blocks.length" orientation="responsive" data-testid="ade-step-mover">
    <FieldLabel>{{ t('space.common.step.label') }}</FieldLabel>
    <div class="flex min-w-0 items-center gap-1.5">
      <TooltipIconButton
        :icon="ICON.back"
        :label="reason || t('space.ade.stepMover.back.tooltip')"
        :aria-label="t('space.ade.stepMover.back.tooltip')"
        :disabled="live || !prev"
        disabled-trigger
        data-testid="ade-step-back"
        @click="move(prev?.id ?? '')"
      />
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <PickerTrigger
            size="kira"
            :label="options[at]?.name"
            :disabled="live"
            data-testid="ade-step-pick"
          />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" data-testid="ade-step-menu">
          <DropdownMenuItem
            v-for="o in options"
            :key="o.id"
            :data-testid="`ade-step-option-${o.id}`"
            :disabled="o.skipped"
            @select="move(o.id)"
          >
            {{ o.name
            }}<span v-if="o.skipped" class="text-muted-foreground">
              {{ t('common.skipped.status') }}</span
            >
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <TooltipIconButton
        icon="arrow-right"
        :label="reason || t('space.ade.stepMover.next.tooltip')"
        :aria-label="t('space.ade.stepMover.next.tooltip')"
        :disabled="live || !next"
        disabled-trigger
        data-testid="ade-step-next"
        @click="move(next?.id ?? '')"
      />
    </div>
  </Field>
</template>
