<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { FieldError } from '@theme/components/ui/field';
import { InputGroup, InputGroupInput } from '@theme/components/ui/input-group';
import { Label } from '@theme/components/ui/label';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowVariants } from '@theme/lib/rowVariants';
import { useInlineEdit } from '@workbench/behaviour/inlineEdit';
import QueryState from '@workbench/components/QueryState.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, ref, watch } from 'vue';
import { control } from '../../bridge/control';
import type { StreamTabRecord } from '../../state/tabDomain';
import DateTimePicker from '../shared/DateTimePicker.vue';
import StreamFilterHistoryMenu from './StreamFilterHistoryMenu.vue';
import { useStreamViewStore } from './state';

// Kafka-only positioning filters. A fresh browse always restarts (applyStreamFilter); a
// token-continued page ignores them. MainView keys the view by tab.id, so plain initializers
// cover "just opened" and "tab changed".
const props = defineProps<{ tab: StreamTabRecord }>();

const streamViewStore = useStreamViewStore();

const offsetEdit = useInlineEdit({
  source: () => props.tab.state.offsetFilter ?? '',
  allowEmpty: true,
  commit: onApplyFilter,
});
const offsetText = offsetEdit.text;
const timestampEdit = useInlineEdit({
  source: () => props.tab.state.timestampFilter ?? '',
  allowEmpty: true,
  commit: onApplyFilter,
});
const timestampText = timestampEdit.text;
const selectedPartitions = ref<number[]>([...props.tab.state.partitions]);

// Unapplied input lives in the runtime store (P375): tab-next/tab-prev switch tabs without a
// blur. Seed from a kept draft, then mirror edits back; `null` means equal to the committed value.
const draftRt = streamViewStore.runtime[props.tab.id];
if (draftRt?.offsetDraft != null) offsetText.value = draftRt.offsetDraft;
if (draftRt?.timestampDraft != null) timestampText.value = draftRt.timestampDraft;
if (draftRt?.partitionsDraft != null) selectedPartitions.value = [...draftRt.partitionsDraft];
watch(
  () => [offsetText.value, props.tab.state.offsetFilter] as const,
  ([text, committed]) =>
    streamViewStore.setFilterDraft(props.tab.id, {
      offset: text === (committed ?? '') ? null : text,
    }),
);
watch(
  () => [timestampText.value, props.tab.state.timestampFilter] as const,
  ([text, committed]) =>
    streamViewStore.setFilterDraft(props.tab.id, {
      timestamp: text === (committed ?? '') ? null : text,
    }),
);
watch(
  () => [[...selectedPartitions.value].sort((a, b) => a - b), props.tab.state.partitions] as const,
  ([selected, committed]) =>
    streamViewStore.setFilterDraft(props.tab.id, {
      partitions:
        selected.length === committed.length && selected.every((p, i) => p === committed[i])
          ? null
          : selected,
    }),
);

function currentFilterInput() {
  return {
    offset: offsetText.value.trim() === '' ? null : offsetText.value.trim(),
    partitions: [...selectedPartitions.value].sort((a, b) => a - b),
    timestamp: timestampText.value.trim() === '' ? null : timestampText.value.trim(),
  };
}

// An unparseable "since" must never reach the read: Date.parse would give NaN and the browse
// would silently start at the low watermark while the invalid text looks applied.
const timestampError = ref<string | null>(null);
function validateTimestamp(): boolean {
  const text = timestampText.value.trim();
  if (text === '' || !Number.isNaN(Date.parse(text))) {
    timestampError.value = null;
    return true;
  }
  timestampError.value = t('studio.stream.timestamp.invalid.error');
  return false;
}

async function onApplyFilter(): Promise<void> {
  if (!validateTimestamp()) return;
  await streamViewStore.applyStreamFilter(props.tab.id, currentFilterInput());
}

async function onClearFilter(): Promise<void> {
  offsetText.value = '';
  selectedPartitions.value = [];
  timestampText.value = '';
  timestampError.value = null;
  await streamViewStore.applyStreamFilter(props.tab.id, {
    offset: null,
    partitions: [],
    timestamp: null,
  });
}

function onApplyFromHistory(
  offset: string | null,
  partitions: number[],
  timestamp: string | null,
): void {
  offsetText.value = offset ?? '';
  selectedPartitions.value = [...partitions];
  timestampText.value = timestamp ?? '';
  timestampError.value = null;
  void streamViewStore.applyStreamFilter(props.tab.id, { offset, partitions, timestamp });
}

// toISOString, not a shape-preserving encoding: the field feeds a query with no original
// spelling to keep, and state.ts load() passes it straight to Date.parse.
const timestampCalendarOpen = ref(false);
const timestampCalendarTriggerEl = ref<{ $el: HTMLElement } | null>(null);
const timestampPickerDate = computed(() => {
  const ms = Date.parse(timestampText.value.trim());
  return Number.isNaN(ms) ? new Date() : new Date(ms);
});
function onPickTimestamp(date: Date): void {
  timestampText.value = date.toISOString();
  void onApplyFilter();
}

const filterHistoryOpen = ref(false);
const partitionMenuOpen = ref(false);
const partitionTriggerEl = ref<{ $el: HTMLElement } | null>(null);

// A topic's path resolves to its partition list one level down; refetched on every open so a
// changed partition count shows.
const partitionsQuery = useQuery(() => ({
  queryKey: ['streamPartitions', props.tab.connectionId, props.tab.path] as const,
  queryFn: async () => {
    const result = await control.treeChildren(
      props.tab.connectionId as string,
      props.tab.path,
      false,
    );
    return result.nodes
      .map((n) => Number(n.name))
      .filter((n) => Number.isInteger(n))
      .sort((a, b) => a - b);
  },
  enabled: partitionMenuOpen.value && props.tab.connectionId !== null,
  staleTime: 0,
}));

function onTogglePartition(p: number): void {
  const idx = selectedPartitions.value.indexOf(p);
  if (idx >= 0) selectedPartitions.value.splice(idx, 1);
  else selectedPartitions.value.push(p);
  void onApplyFilter();
}

const partitionButtonLabel = computed(() => {
  const n = selectedPartitions.value.length;
  if (n === 0) return t('studio.stream.allPartitions.label');
  if (n === 1)
    return t('studio.stream.partition.label', { id: String(selectedPartitions.value[0]) });
  return t('studio.stream.partitions.label', { count: n });
});
</script>

<template>
  <ViewToolbar>
    <div class="relative">
      <TooltipIconButton
        :icon="ICON.history"
        :label="t('studio.common.filterHistory.placeholder')"
        data-testid="stream-filter-history-button"
        @click="filterHistoryOpen = !filterHistoryOpen"
      />
      <StreamFilterHistoryMenu
        v-if="filterHistoryOpen"
        :tab-id="tab.id"
        @apply="onApplyFromHistory"
        @close="filterHistoryOpen = false"
      />
    </div>
    <div class="w-40 shrink-0">
      <InputGroup variant="kira" class="flex w-full font-data">
        <span
          class="shrink-0 text-kira-sm"
          :class="tab.state.offsetFilter ? 'text-state-on' : 'text-muted-foreground'"
          >{{
            t('studio.stream.offset.label')
          }}</span
        >
        <InputGroupInput
          :model-value="offsetText"
          :placeholder="t('studio.stream.offset.placeholder')"
          class="h-full p-0 font-data"
          data-testid="stream-filter-offset"
          @update:model-value="offsetEdit.onInput"
          @keydown="offsetEdit.onKeydown"
          @blur="offsetEdit.onCommit"
        />
      </InputGroup>
    </div>
    <div class="relative">
      <Tooltip>
        <TooltipTrigger as-child>
          <Button
            ref="partitionTriggerEl"
            variant="toolbar"
            size="kira"
            data-testid="stream-filter-partition"
            :class="selectedPartitions.length ? 'text-state-on' : ''"
            @click="partitionMenuOpen = !partitionMenuOpen"
          >
            <CodiconIcon :name="ICON.filter" :size="13" />
            {{ partitionButtonLabel }}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{{ t('studio.stream.partitionFilter.tooltip') }}</TooltipContent>
      </Tooltip>
      <Popover :open="partitionMenuOpen" @update:open="(v) => (partitionMenuOpen = v)">
        <PopoverAnchor
          :reference="(partitionTriggerEl?.$el as HTMLElement) ?? undefined"
          class="hidden"
        />
        <PopoverContent align="start" class="w-56 p-0" data-testid="stream-partition-menu">
          <div class="flex flex-col max-h-60 overflow-y-auto gap-0.5 p-1">
            <QueryState :query="partitionsQuery" variant="inline">
              <template #empty>
                <InlineState kind="empty">{{ t('studio.stream.noPartitions.empty') }}</InlineState>
              </template>
              <template #default="{ data }">
                <Label
                  v-for="p in data"
                  :key="p"
                  class="partition-option"
                  :class="rowVariants()"
                  :data-testid="`stream-filter-partition-option-${p}`"
                >
                  <Checkbox
                    class="size-3.5"
                    :model-value="selectedPartitions.includes(p)"
                    @update:model-value="onTogglePartition(p)"
                  >
                    <CodiconIcon name="check" :size="12" />
                  </Checkbox>
                  <span>{{ t('studio.stream.partition.label', { id: String(p) }) }}</span>
                </Label>
              </template>
            </QueryState>
          </div>
        </PopoverContent>
      </Popover>
    </div>
    <div class="flex flex-col gap-0.5 shrink-0">
      <div class="flex items-center gap-0.5">
        <Tooltip :disabled="!timestampError">
          <TooltipTrigger as-child>
            <InputGroup variant="kira" class="flex w-40 font-data" :aria-invalid="!!timestampError">
              <span
                class="shrink-0 text-kira-sm"
                :class="tab.state.timestampFilter ? 'text-state-on' : 'text-muted-foreground'"
                >{{
                  t('studio.stream.since.label')
                }}</span
              >
              <InputGroupInput
                :model-value="timestampText"
                :placeholder="t('studio.cellEditor.detect.iso8601.status')"
                class="h-full p-0 font-data"
                data-testid="stream-filter-timestamp"
                @update:model-value="timestampEdit.onInput"
                @keydown="timestampEdit.onKeydown"
                @blur="timestampEdit.onCommit"
              />
            </InputGroup>
          </TooltipTrigger>
          <TooltipContent>{{ timestampError }}</TooltipContent>
        </Tooltip>
        <span class="relative shrink-0">
          <TooltipIconButton
            icon="calendar"
            :label="t('studio.common.pickDateTime.tooltip')"
            ref="timestampCalendarTriggerEl"
            data-testid="stream-filter-timestamp-calendar"
            @click="timestampCalendarOpen = !timestampCalendarOpen"
          />
          <Popover :open="timestampCalendarOpen" @update:open="(v) => (timestampCalendarOpen = v)">
            <PopoverAnchor
              :reference="(timestampCalendarTriggerEl?.$el as HTMLElement) ?? undefined"
              class="hidden"
            />
            <PopoverContent
              align="start"
              class="w-56 p-0"
              data-testid="stream-filter-timestamp-calendar-popover"
            >
              <DateTimePicker
                :model-value="timestampPickerDate"
                zone="local"
                @update:model-value="onPickTimestamp"
              />
            </PopoverContent>
          </Popover>
        </span>
      </div>
      <FieldError
        v-if="timestampError"
        class="whitespace-nowrap"
        data-testid="stream-filter-timestamp-error"
        >{{
          timestampError
        }}</FieldError
      >
    </div>
    <Tooltip>
      <TooltipTrigger as-child>
        <Button
          variant="toolbar"
          size="kira"
          data-testid="stream-filter-clear"
          @click="onClearFilter"
        >
          {{ t('common.action.clear.label') }}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{{ t('studio.common.clearFilters.tooltip') }}</TooltipContent>
    </Tooltip>
  </ViewToolbar>
</template>
