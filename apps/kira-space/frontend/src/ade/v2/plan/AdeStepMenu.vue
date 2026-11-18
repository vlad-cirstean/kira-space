<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import MenuItems from '@workbench/components/MenuItems.vue';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed } from 'vue';
import { useStepMoves } from '../panel/useStepMoves';
import type { CardModel } from './usePlanModel';

// The current step as text on the card; click moves the task to another step (each move opens the
// stage dialog). No tooltip: a Tooltip between DropdownMenu and its trigger steals the popper anchor.
const props = defineProps<{ card: CardModel }>();

const { options, at, prev, next, live, reason, move } = useStepMoves(() => props.card);

const moveItems = computed<MenuItem[]>(() => [
  ...(live.value ? [{ type: 'label' as const, label: reason.value }] : []),
  {
    type: 'item',
    id: 'back',
    label: t('space.ade.stepMover.back.tooltip'),
    icon: 'arrow-left',
    disabled: live.value || !prev.value,
    run: () => move(prev.value?.id ?? ''),
  },
  {
    type: 'item',
    id: 'next',
    label: t('space.ade.stepMover.next.tooltip'),
    icon: 'arrow-right',
    disabled: live.value || !next.value,
    run: () => move(next.value?.id ?? ''),
  },
  { type: 'separator' },
  {
    type: 'radio',
    id: 'step',
    value: options.value[at.value]?.id ?? null,
    items: options.value.map((o) => ({
      type: 'option',
      id: o.id,
      testId: `ade-row-step-option-${o.id}`,
      value: o.id,
      label: o.name,
      disabled: live.value || o.skipped,
      hint: o.skipped ? t('common.skipped.status') : undefined,
      run: () => move(o.id),
    })),
  },
]);
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
      <MenuItems :items="moveItems" />
    </DropdownMenuContent>
  </DropdownMenu>
</template>
