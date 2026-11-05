<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import AdeTip from '../AdeTip.vue';
import type { NeedsItem } from '../board/needsYou';
import { useNeedsAction } from '../needs/useNeedsAction';
import { SOLID_CLASS } from '../tones';

// The `!` circle: something here needs you. Click does the item's action: Take over a stuck run,
// open the waiting session.
const props = defineProps<{ tip: string; item?: NeedsItem | null }>();
const { perform, busy } = useNeedsAction();

function onClick(): void {
  if (props.item) void perform(props.item);
}
</script>

<template>
  <AdeTip :text="tip">
    <Button
      size="inline"
      type="button"
      variant="toolbar"
      :aria-label="t('common.needsYou.status')"
      class="size-4.5 rounded-full border-0 p-0 text-kira-sm font-medium"
      :class="SOLID_CLASS.amber"
      :pending="busy"
      data-testid="ade-attention"
      @click.stop="onClick"
      >!</Button
    >
  </AdeTip>
</template>
