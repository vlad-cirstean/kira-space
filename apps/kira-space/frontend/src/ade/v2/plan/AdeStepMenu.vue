<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { useStepMoves } from '../panel/useStepMoves';
import type { CardModel } from './usePlanModel';

// The current step as text on the card; click moves the task to another step (each move opens the
// stage dialog). No tooltip: a Tooltip between DropdownMenu and its trigger steals the popper anchor.
const props = defineProps<{ card: CardModel }>();

const { options, at, prev, next, live, reason, move } = useStepMoves(() => props.card);
</script>

<template>
  <DropdownMenu :modal="false">
    <DropdownMenuTrigger as-child>
      <Button
        type="button"
        variant="toolbar"
        size="inline"
        class="h-4.5 min-w-0 max-w-28 shrink justify-start gap-0.5 rounded-kira-sm px-1 text-kira-sm font-normal"
        :class="card.progress.bad ? 'text-tone-red' : 'text-muted-foreground'"
        :aria-label="t('space.ade.plan.card.step.aria', { step: card.progress.label })"
        data-testid="ade-step-label"
        @click.stop
      >
        <span class="truncate">{{ card.progress.label }}</span>
        <CodiconIcon name="chevron-down" :size="12" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="start" data-testid="ade-row-step-menu" @click.stop>
      <DropdownMenuLabel v-if="live">{{ reason }}</DropdownMenuLabel>
      <DropdownMenuItem :disabled="live || !prev" @select="move(prev?.id ?? '')">
        {{ t('space.ade.stepMover.back.tooltip') }}
      </DropdownMenuItem>
      <DropdownMenuItem :disabled="live || !next" @select="move(next?.id ?? '')">
        {{ t('space.ade.stepMover.next.tooltip') }}
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuRadioGroup :model-value="options[at]?.id ?? ''">
        <DropdownMenuRadioItem
          v-for="o in options"
          :key="o.id"
          :value="o.id"
          :disabled="live || o.skipped"
          :data-testid="`ade-row-step-option-${o.id}`"
          @select="move(o.id)"
        >
          {{ o.name
          }}<span v-if="o.skipped" class="text-muted-foreground">
            {{ t('common.skipped.status') }}</span
          >
          <template #indicator-icon><CodiconIcon name="check" :size="13" /></template>
        </DropdownMenuRadioItem>
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
