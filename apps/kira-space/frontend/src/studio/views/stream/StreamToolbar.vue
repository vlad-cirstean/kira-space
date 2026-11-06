<script setup lang="ts">
import {
  SQS_MAX_VISIBILITY_TIMEOUT_SECONDS,
  SQS_MIN_VISIBILITY_TIMEOUT_SECONDS,
} from '@shared/domain/streamFilter';
import type { PageSize } from '@shared/domain/tabs';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { FieldError } from '@theme/components/ui/field';
import { InputGroup, InputGroupInput } from '@theme/components/ui/input-group';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import RunState from '@theme/RunState.vue';
import { formatCount } from '@workbench/behaviour/format';
import { useInlineEdit } from '@workbench/behaviour/inlineEdit';
import { errorText } from '@workbench/bridge/codedError';
import ToolbarSeparator from '@workbench/components/ToolbarSeparator.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref, watch } from 'vue';
import type { StreamTabRecord } from '../../../state/tabDomain';
import { useTabsStore } from '../../../state/tabs';
import { useRunState } from '../../state/runState';
import { pageSizeOptions } from '../shared/page/sizes';
import { deleteSqsMessage } from './mutations';
import StreamComposeMessage from './StreamComposeMessage.vue';
import { useStreamViewStore } from './state';
import { redriveSentence, useStreamGate } from './useStreamGate';

const props = defineProps<{ tab: StreamTabRecord }>();
const emit = defineEmits<{ refresh: [] }>();

const confirmDialogStore = useConfirmDialogStore();
const streamViewStore = useStreamViewStore();
const tabsStore = useTabsStore();
const gate = useStreamGate(() => props.tab);
const { rt, running, isBatch, isKafka, isReadOnlyBatch } = gate;
const runState = useRunState(() => props.tab.id);

const visibilityRangeHint = computed(() =>
  t('studio.stream.visibilityRange.hint', {
    min: SQS_MIN_VISIBILITY_TIMEOUT_SECONDS,
    max: SQS_MAX_VISIBILITY_TIMEOUT_SECONDS,
  }),
);
function validVisibility(text: string): boolean {
  const trimmed = text.trim();
  const n = Number(trimmed);
  return (
    /^\d+$/.test(trimmed) &&
    n >= SQS_MIN_VISIBILITY_TIMEOUT_SECONDS &&
    n <= SQS_MAX_VISIBILITY_TIMEOUT_SECONDS
  );
}
const visibility = useInlineEdit({
  source: () => String(props.tab.state.sqsVisibilityTimeoutSeconds),
  commit: (text) => {
    if (!validVisibility(text)) throw new Error(visibilityRangeHint.value);
    streamViewStore.setPollVisibilityTimeout(props.tab.id, Number(text));
  },
});
const visibilityValid = computed(() => validVisibility(visibility.text.value));

// A read-only SQS poll hides messages for the chosen time and raises their receive count: ask
// once per tab before polling.
async function onPoll(): Promise<void> {
  if (isReadOnlyBatch.value && !visibilityValid.value) return;
  if (isReadOnlyBatch.value && !rt.value?.receiveAcknowledged) {
    const limit =
      gate.redriveLimit.value ?? (await streamViewStore.fetchRedriveLimit(props.tab.id));
    const ok = await confirmDialogStore.confirm({
      title: t('studio.stream.pollConfirm.title'),
      message: [
        t('studio.stream.pollConfirm.body', { duration: gate.hideDuration.value }),
        redriveSentence(limit),
      ].join(' '),
      confirmLabel: t('studio.stream.poll.label'),
      tone: 'primary',
    });
    if (!ok) return;
    streamViewStore.acknowledgeReceive(props.tab.id);
  }
  void streamViewStore.poll(props.tab.id);
}

const statusLine = computed(() => {
  const r = rt.value;
  if (!r) return '';
  const parts: string[] = [t('studio.stream.rowsOnPage.status', { count: r.rowCount })];
  if (r.count) {
    parts.push(
      t('studio.stream.total.status', {
        approx: r.count.exact ? '' : '~',
        total: formatCount(r.count.value),
      }),
    );
  }
  return parts.join(' · ');
});

// The page-size ceiling is per connection, so the options exist only once caps arrive.
const pageSizes = computed(() => pageSizeOptions('stream-', gate.caps.value?.maxPageSize));
const PAGE_SIZE_ITEMS = computed(() =>
  pageSizes.value.map((o) => ({ value: String(o.value), label: o.label, testid: o.testid })),
);

// A persisted pageSize above the cap (or from another engine) snaps to the largest offered size:
// the segmented control renders nothing selected when its value is not among the options.
watch(
  pageSizes,
  (options) => {
    if (options.length === 0 || options.some((o) => o.value === props.tab.state.pageSize)) return;
    const largest = options[options.length - 1];
    if (largest) tabsStore.patchStreamTabState(props.tab.id, { pageSize: largest.value });
  },
  { immediate: true },
);

const composeOpen = ref(false);
const addMessageTriggerEl = ref<{ $el: HTMLElement } | null>(null);

const hasSelectedRow = computed(
  () => rt.value?.selectedRow !== null && rt.value?.selectedRow !== undefined,
);

const deleteLabel = computed(() => {
  if (gate.stale.value) return t('studio.common.connectionLost.tooltip');
  return t(
    hasSelectedRow.value
      ? 'studio.stream.deleteSelected.tooltip'
      : 'studio.stream.selectFirst.tooltip',
  );
});

// SQS-only Delete targets the selected row; the context menu stays copy-only for every engine.
async function onDeleteMessage(): Promise<void> {
  const selectedRow = rt.value?.selectedRow;
  if (selectedRow === null || selectedRow === undefined) return;
  const row = gate.rowAt(selectedRow);
  if (!row?.key) return;
  const confirmed = await confirmDialogStore.confirm({
    title: t('studio.stream.deleteConfirm.title'),
    message: [
      t('studio.stream.deleteConfirm.body', { id: row.key }),
      t('common.confirm.irreversible.body'),
    ].join(' '),
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
  });
  if (!confirmed) return;
  if (gate.stale.value) {
    streamViewStore.setActionError(props.tab.id, t('studio.common.connectionLost.error'));
    return;
  }
  try {
    await deleteSqsMessage(props.tab.id, row.key);
    streamViewStore.setActionError(props.tab.id, null);
  } catch (err) {
    streamViewStore.setActionError(props.tab.id, errorText(err));
  }
}
</script>

<template>
  <ViewToolbar>
    <div class="flex items-center gap-1.5 min-w-0">
      <TooltipIconButton
        :icon="ICON.refresh"
        :label="t('common.action.refresh.label')"
        disabled-trigger
        data-testid="stream-refresh"
        :disabled="isBatch"
        @click="emit('refresh')"
      />
      <TooltipIconButton
        :icon="ICON.stop"
        :label="t('common.action.stop.label')"
        disabled-trigger
        :class="{ 'text-error': running }"
        data-testid="stream-stop"
        :disabled="!running"
        @click="streamViewStore.stop(tab.id)"
      />
    </div>
    <ToolbarSeparator />

    <div class="flex items-center gap-1.5 min-w-0">
      <Tooltip>
        <TooltipTrigger as-child>
          <Button
            variant="toolbar"
            size="kira-icon"
            :class="rt?.countError ? 'text-error' : ''"
            :aria-label="t('studio.stream.count.label')"
            data-testid="stream-count"
            @click="streamViewStore.runCount(tab.id)"
          >
            <CodiconIcon name="symbol-number" :size="13" />
          </Button>
        </TooltipTrigger>
        <TooltipContent data-testid="stream-count-tooltip">{{
          rt?.countError
            ? t('studio.common.countFailed.tooltip', { error: rt.countError })
            : t('studio.stream.count.label')
        }}</TooltipContent>
      </Tooltip>
      <span class="text-kira-sm text-muted-foreground" data-testid="stream-status">{{
        statusLine
      }}</span>
      <Tooltip v-if="isReadOnlyBatch">
        <TooltipTrigger as-child>
          <InputGroup
            variant="kira"
            class="flex w-24 shrink-0 font-data"
            :aria-invalid="!visibilityValid"
          >
            <span class="shrink-0 text-kira-sm text-muted-foreground">{{
              t('common.action.hide.label')
            }}</span>
            <InputGroupInput
              :model-value="visibility.text.value"
              type="number"
              inputmode="numeric"
              :min="SQS_MIN_VISIBILITY_TIMEOUT_SECONDS"
              :max="SQS_MAX_VISIBILITY_TIMEOUT_SECONDS"
              step="1"
              class="h-full p-0 font-data"
              data-testid="stream-poll-visibility"
              @update:model-value="visibility.onInput"
              @keydown="visibility.onKeydown"
              @blur="visibility.onCommit"
            />
            <span class="shrink-0 text-kira-sm text-muted-foreground">{{
              t('format.duration.secondsCompact', { seconds: '' })
            }}</span>
          </InputGroup>
        </TooltipTrigger>
        <TooltipContent>{{ t('studio.stream.hideDuration.description') }}</TooltipContent>
      </Tooltip>
      <FieldError
        v-if="isReadOnlyBatch && !visibilityValid"
        data-testid="stream-poll-visibility-error"
        >{{
          visibilityRangeHint
        }}</FieldError
      >
      <Tooltip v-if="isBatch">
        <TooltipTrigger as-child>
          <Button
            variant="dialog-primary"
            size="kira"
            data-testid="stream-poll"
            :disabled="isReadOnlyBatch && !visibilityValid"
            @click="onPoll"
          >
            <CodiconIcon name="arrow-swap" :size="13" />
            {{ t('studio.stream.poll.label') }}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{{
          isReadOnlyBatch && !visibilityValid
            ? visibilityRangeHint
            : t('studio.stream.poll.tooltip')
        }}</TooltipContent>
      </Tooltip>
      <TooltipIconButton
        v-else
        icon="arrow-right"
        :label="t('common.action.nextPage.label')"
        data-testid="stream-next"
        disabled-trigger
        :disabled="!rt?.hasMore"
        @click="streamViewStore.goNext(tab.id)"
      />
    </div>

    <ToolbarSeparator />

    <SecondaryTabs
      variant="segmented"
      :model-value="String(tab.state.pageSize)"
      :items="PAGE_SIZE_ITEMS"
      data-testid="stream-page-size-picker"
      @update:model-value="(v) => streamViewStore.setPageSize(tab.id, Number(v) as PageSize)"
    />

    <ToolbarSeparator />

    <div class="flex items-center gap-1.5 min-w-0">
      <div class="relative">
        <TooltipIconButton
          v-if="gate.canInsertCaps.value"
          :icon="ICON.add"
          :label="
            gate.stale.value
              ? t('studio.common.connectionLost.tooltip')
              : t(isKafka ? 'studio.stream.produce.label' : 'studio.stream.send.label')
          "
          disabled-trigger
          :disabled="gate.stale.value"
          :aria-label="t('studio.stream.addMessage.aria')"
          ref="addMessageTriggerEl"
          data-testid="stream-add-message"
          @click="composeOpen = !composeOpen"
        />
        <Popover
          :open="composeOpen && gate.canInsert.value"
          @update:open="(v) => (composeOpen = v)"
        >
          <PopoverAnchor
            :reference="(addMessageTriggerEl?.$el as HTMLElement) ?? undefined"
            class="hidden"
          />
          <PopoverContent align="end" class="w-96 p-0" data-testid="stream-add-message-panel">
            <StreamComposeMessage
              :tab-id="tab.id"
              :kind="isKafka ? 'kafka' : 'sqs'"
              @close="composeOpen = false"
            />
          </PopoverContent>
        </Popover>
      </div>
      <TooltipIconButton
        v-if="gate.canDeleteCaps.value"
        :icon="ICON.delete"
        :label="deleteLabel"
        :aria-label="t('studio.stream.deleteMessage.aria')"
        disabled-trigger
        :disabled="!hasSelectedRow || gate.stale.value"
        data-testid="stream-delete-message"
        @click="onDeleteMessage"
      />
      <TooltipIconButton
        :icon="ICON.search"
        :label="t('studio.common.searchPage.label')"
        :pressed="!!rt?.searchOpen"
        data-testid="stream-search-toggle"
        @click="streamViewStore.toggleSearchOpen(tab.id)"
      />
    </div>
    <span class="ml-auto" />
    <Tooltip :disabled="true">
      <TooltipTrigger as-child>
        <RunState :state="runState" />
      </TooltipTrigger>
    </Tooltip>
  </ViewToolbar>
</template>
