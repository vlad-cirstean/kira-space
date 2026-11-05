<script setup lang="ts">
import type { KibanaTimePreset, KibanaTimeSelection } from '@shared/domain/kibana';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { formatDateTime } from '@theme/format';
import { type MessageKey, t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { computed, ref, watch } from 'vue';
import { KIBANA_CALENDAR_PRESETS, KIBANA_RELATIVE_PRESETS, resolveTime } from './time';

const model = defineModel<KibanaTimeSelection>({ required: true });
defineProps<{ disabled?: boolean }>();
const emit = defineEmits<{ refresh: [] }>();

const open = ref(false);
const from = ref('');
const to = ref('');

// `<input type="datetime-local">` speaks local time without a zone.
function toLocalInput(iso: string): string {
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

watch(open, (o) => {
  if (!o) return;
  const range = resolveTime(model.value);
  from.value = toLocalInput(range.from);
  to.value = toLocalInput(range.to);
});

const PRESET_KEYS: Record<KibanaTimePreset, MessageKey> = {
  last15m: 'studio.kibana.time.last15m.option',
  last30m: 'studio.kibana.time.last30m.option',
  last1h: 'studio.kibana.time.last1h.option',
  last4h: 'studio.kibana.time.last4h.option',
  last12h: 'studio.kibana.time.last12h.option',
  last24h: 'studio.kibana.time.last24h.option',
  last7d: 'studio.kibana.time.last7d.option',
  last30d: 'studio.kibana.time.last30d.option',
  last90d: 'studio.kibana.time.last90d.option',
  last1y: 'studio.kibana.time.last1y.option',
  today: 'studio.kibana.time.today.option',
  thisWeek: 'studio.kibana.time.thisWeek.option',
  thisMonth: 'studio.kibana.time.thisMonth.option',
  thisYear: 'studio.kibana.time.thisYear.option',
};

const label = computed(() => {
  const sel = model.value;
  if (sel.mode === 'preset') return t(PRESET_KEYS[sel.preset]);
  return `${formatDateTime(sel.from)} – ${formatDateTime(sel.to)}`;
});

const absoluteValid = computed(() => {
  const a = new Date(from.value).getTime();
  const b = new Date(to.value).getTime();
  return Number.isFinite(a) && Number.isFinite(b) && a < b;
});

function pick(preset: KibanaTimePreset): void {
  model.value = { mode: 'preset', preset };
  open.value = false;
}

function applyAbsolute(): void {
  if (!absoluteValid.value) return;
  model.value = {
    mode: 'absolute',
    from: new Date(from.value).toISOString(),
    to: new Date(to.value).toISOString(),
  };
  open.value = false;
}

const isActive = (preset: KibanaTimePreset): boolean =>
  model.value.mode === 'preset' && model.value.preset === preset;
</script>

<template>
  <div class="flex items-center gap-1">
    <Popover v-model:open="open">
      <PopoverTrigger as-child>
        <Button variant="toolbar" size="kira" :disabled="disabled" data-testid="kibana-time-picker">
          <CodiconIcon name="calendar" :size="13" />
          <span class="truncate">{{ label }}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" class="w-80 p-2" data-testid="kibana-time-popover">
        <div class="grid grid-cols-2 gap-x-2 gap-y-0.5">
          <button
            v-for="p in [...KIBANA_RELATIVE_PRESETS, ...KIBANA_CALENDAR_PRESETS]"
            :key="p"
            type="button"
            :class="rowVariants({ layout: 'menu', selected: isActive(p) })"
            :aria-pressed="isActive(p)"
            :data-testid="`kibana-time-preset-${p}`"
            @click="pick(p)"
          >
            {{ t(PRESET_KEYS[p]) }}
          </button>
        </div>
        <div class="mt-2 flex flex-col gap-1.5 pt-2">
          <div class="flex items-center gap-2 text-kira-sm text-muted-foreground">
            <span class="w-10 shrink-0">{{ t('studio.kibana.time.from.label') }}</span>
            <Input
              v-model="from"
              type="datetime-local"
              :aria-label="t('studio.kibana.time.from.label')"
              data-testid="kibana-time-from"
            />
          </div>
          <div class="flex items-center gap-2 text-kira-sm text-muted-foreground">
            <span class="w-10 shrink-0">{{ t('studio.kibana.time.to.label') }}</span>
            <Input
              v-model="to"
              type="datetime-local"
              :aria-label="t('studio.kibana.time.to.label')"
              data-testid="kibana-time-to"
            />
          </div>
          <Button
            variant="dialog-primary"
            size="kira"
            class="self-end"
            :disabled="!absoluteValid"
            data-testid="kibana-time-apply"
            @click="applyAbsolute"
          >
            {{ t('studio.kibana.time.apply.label') }}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
    <Button
      variant="toolbar"
      size="kira"
      :disabled="disabled"
      :aria-label="t('studio.kibana.time.refresh.aria')"
      data-testid="kibana-time-refresh"
      @click="emit('refresh')"
    >
      <CodiconIcon name="refresh" :size="13" />
    </Button>
  </div>
</template>
