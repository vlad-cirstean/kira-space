<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Input } from '@theme/components/ui/input';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import {
  Tooltip,
  TooltipContent,
  TooltipDisabledTrigger,
  TooltipTrigger,
} from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { useFocus } from '@vueuse/core';
import { computed, nextTick, ref, watch } from 'vue';
import DateTimePicker from '../DateTimePicker.vue';
import { type CellFormat, formatLabel } from './formats';
import TimestampReading from './TimestampReading.vue';
import {
  defaultShapeFor,
  describeTimestamp,
  encodeTimestamp,
  fromEditableText,
  parseTimestamp,
  toEditableText,
} from './timestamp';

// The translate pane for the three timestamp formats — the sibling of the
// hex/base64 decoded pane (CellEditorView.vue hosts both under the same head/body/staging
// shape). Bidirectional and live: every keystroke here re-encodes into `doc`, and every change to
// `doc` (hand-typed in the main editor, Reset, a fresh cell) re-renders this pane — never a
// one-shot commit. Stages nothing itself; staging is `.editor-body`'s existing focusout/
// Ctrl+Enter rule, which this pane inherits by living inside it.
const props = defineProps<{ format: CellFormat; readonly?: boolean }>();
const doc = defineModel<string>('doc', { required: true });

const zone = ref<'local' | 'utc'>('local');
function setZone(v: string): void {
  zone.value = v as 'local' | 'utc';
}
const FIELD_PATTERN = 'YYYY-MM-DD HH:mm:ss';
const zoneOptions = computed(() => [
  {
    value: 'local' as const,
    label: t('studio.cellEditor.timestamp.zoneLocal.label'),
    testid: 'cell-editor-timestamp-zone-local',
  },
  {
    value: 'utc' as const,
    label: t('studio.cellEditor.timestamp.zoneUtc.label'),
    testid: 'cell-editor-timestamp-zone-utc',
  },
]);

const parsed = computed(() => parseTimestamp(props.format, doc.value));
const reading = computed(() => describeTimestamp(props.format, doc.value));

function shapeOrDefault() {
  return (
    parsed.value?.shape ??
    defaultShapeFor(props.format as 'epochSeconds' | 'epochMillis' | 'iso8601')
  );
}

// The editable field's own text — a plain ref, not a computed off `doc`, so a keystroke that
// doesn't yet parse (mid-typing "2024-01-1") is never clobbered before the user finishes it.
// Re-synced from `doc`/`zone` whenever either changes from *outside* this field (the encoded pane,
// Reset, a fresh cell, the zone switch) — and only when the recomputed text differs, so
// a round-tripped edit (parse this field's own text -> encode -> doc -> reparse -> re-render)
// never touches the ref, and so never disturbs the caret, when it lands back on the same text.
const fieldText = ref('');
function syncFieldFromDoc(): void {
  const p = parsed.value;
  const next = p ? toEditableText(p.date, zone.value, p.shape.fractionDigits) : '';
  if (next !== fieldText.value) fieldText.value = next;
}
watch(() => [doc.value, props.format] as const, syncFieldFromDoc, { immediate: true });
watch(zone, syncFieldFromDoc);

function onFieldInput(text: string): void {
  fieldText.value = text;
  const date = fromEditableText(text, zone.value);
  if (!date) return; // incomplete/invalid mid-typing — wait for more input, emit nothing (D15)
  doc.value = encodeTimestamp(shapeOrDefault(), date);
}

function onPick(date: Date): void {
  doc.value = encodeTimestamp(shapeOrDefault(), date);
}

const calendarOpen = ref(false);
const calendarAnchorRef = ref<HTMLElement | null>(null);
const calendarTriggerEl = ref<{ $el: HTMLElement } | null>(null);
const pickerDate = computed(() => parsed.value?.date ?? new Date());

// The day cell that stages a pick lives inside PopoverContent, which reka teleports to
// document.body — outside `.editor-body`'s own DOM subtree. Its own onEditorBlur only stages when
// focus crosses that subtree's boundary (`container.contains(next)`), so once the day cell (never
// itself inside `.editor-body`) is removed from the DOM on close, the browser drops focus with no
// element inside `.editor-body` to blur *from* — the next explicit focus (format/status/etc.)
// fires no focusout at all, and the pick is silently lost (cell-editor.spec.ts's day-pick-then-
// blur scenario caught it). Returning focus to this trigger on close (the old, non-teleported
// PopoverPanel's own implicit behavior) restores a real element inside `.editor-body` to blur from.
const { focused: triggerFocused } = useFocus(computed(() => calendarTriggerEl.value?.$el));
watch(calendarOpen, (open) => {
  if (!open) void nextTick().then(() => (triggerFocused.value = true));
});
</script>

<template>
  <div class="flex flex-col" data-testid="cell-editor-timestamp-pane">
    <Alert class="shrink-0" variant="info" icon="clock" data-testid="cell-editor-timestamp">
      <AlertDescription>
        <div class="flex items-center gap-1">
          <template v-if="reading">
            <TimestampReading
              :subtle="zone !== 'local'"
              data-testid="cell-editor-timestamp-local"
              >{{
                reading.local
              }}</TimestampReading
            >
            <span class="opacity-50">·</span>
            <TimestampReading :subtle="zone !== 'utc'" data-testid="cell-editor-timestamp-utc">{{
              reading.utc
            }}</TimestampReading>
            <span class="opacity-50">·</span>
            <TimestampReading subtle data-testid="cell-editor-timestamp-relative">{{
              reading.relative
            }}</TimestampReading>
          </template>
          <TimestampReading v-else subtle data-testid="cell-editor-timestamp-unparseable">{{
            t('studio.cellEditor.timestamp.unparseable.status', { format: formatLabel(format) })
          }}</TimestampReading>
        </div>
      </AlertDescription>
    </Alert>

    <div class="flex items-center gap-1 py-1.5 px-2">
      <SecondaryTabs
        variant="segmented"
        :model-value="zone"
        :items="zoneOptions"
        data-testid="cell-editor-timestamp-zone"
        @update:model-value="(v) => setZone(v)"
      />
      <div class="flex-1 min-w-0">
        <Tooltip>
          <TooltipTrigger as-child>
            <TooltipDisabledTrigger class="w-full">
              <Input
                :model-value="fieldText"
                :disabled="readonly"
                size="kira"
                class="w-full"
                data-testid="cell-editor-timestamp-field"
                @update:model-value="onFieldInput(String($event))"
              />
            </TooltipDisabledTrigger>
          </TooltipTrigger>
          <TooltipContent>{{
            t('studio.cellEditor.timestamp.field.tooltip', { format: FIELD_PATTERN })
          }}</TooltipContent>
        </Tooltip>
      </div>
      <Popover :open="calendarOpen" @update:open="calendarOpen = $event">
        <span ref="calendarAnchorRef" class="relative shrink-0">
          <TooltipIconButton
            icon="calendar"
            :label="t('studio.common.pickDateTime.tooltip')"
            disabled-trigger
            ref="calendarTriggerEl"
            :disabled="readonly"
            data-testid="cell-editor-timestamp-calendar"
            @click="calendarOpen = !calendarOpen"
          />
          <PopoverAnchor :reference="calendarAnchorRef ?? undefined" />
        </span>
        <PopoverContent
          align="start"
          class="w-56 gap-0 p-0"
          data-testid="cell-editor-timestamp-calendar-popover"
        >
          <DateTimePicker :model-value="pickerDate" :zone="zone" @update:model-value="onPick" />
        </PopoverContent>
      </Popover>
    </div>
  </div>
</template>
