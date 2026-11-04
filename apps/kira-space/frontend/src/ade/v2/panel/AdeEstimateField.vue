<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import { computed, ref, watch } from 'vue';

// Estimate row: number + h/d, written on change; empty clears.
const props = defineProps<{
  /** `Task.est`: `''` none, else `<n>h` / `<n>d`. */
  est: string;
  /** Days the task spans on the plan. */
  days: number;
  error: string | null;
}>();

const emit = defineEmits<{ save: [value: string] }>();

function split(est: string): { num: string; unit: 'h' | 'd' } {
  const m = est.trim().match(/^(\d+(?:\.\d+)?)\s*([hd])$/);
  return m ? { num: m[1] as string, unit: m[2] as 'h' | 'd' } : { num: '', unit: 'h' };
}

const parsed = computed(() => split(props.est));
const num = ref(parsed.value.num);
const unit = ref<'h' | 'd'>(parsed.value.unit);

watch(parsed, (next) => {
  num.value = next.num;
  unit.value = next.unit;
});

function commit(): void {
  const numStr = String(num.value).trim();
  emit('save', numStr ? `${numStr}${unit.value}` : '');
}

function pickUnit(next: 'h' | 'd'): void {
  unit.value = next;
  if (String(num.value).trim()) commit();
}

function clear(): void {
  num.value = '';
  emit('save', '');
}

const unitItems = computed(() => [
  { value: 'h', label: t('space.ade.estimate.hours.option'), testid: 'ade-estimate-unit-h' },
  { value: 'd', label: t('space.ade.estimate.days.option'), testid: 'ade-estimate-unit-d' },
]);

const hint = computed(() =>
  props.days > 1 ? t('space.ade.estimate.spans.label', { count: props.days }) : '',
);
</script>

<template>
  <div class="flex flex-col gap-0.5">
    <div class="flex items-center gap-1.5">
      <Input
        id="ade-est-num"
        v-model="num"
        type="number"
        min="0"
        step="0.5"
        size="kira"
        class="w-20"
        :aria-invalid="error ? true : undefined"
        :aria-describedby="error ? 'ade-estimate-error-text' : undefined"
        data-testid="ade-estimate-num"
        @change="commit"
      />
      <!-- mousedown.prevent: a click would otherwise blur the number input first and commit with the stale unit. -->
      <SecondaryTabs
        variant="segmented"
        size="kira"
        :model-value="unit"
        :items="unitItems"
        :aria-label="t('space.ade.estimate.unit.aria')"
        @mousedown.prevent
        @update:model-value="(v) => pickUnit(v === 'd' ? 'd' : 'h')"
      />
      <TooltipIconButton
        v-if="est !== ''"
        icon="close"
        :label="t('space.ade.estimate.clear.tooltip')"
        data-testid="ade-estimate-clear"
        @click="clear"
      />
      <span :class="rowMeta">{{ hint }}</span>
    </div>
    <FieldError v-if="error" id="ade-estimate-error-text" data-testid="ade-estimate-error">{{
      error
    }}</FieldError>
  </div>
</template>
