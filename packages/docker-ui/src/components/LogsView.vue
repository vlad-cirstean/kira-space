<script setup lang="ts">
import { newUuid } from '@shared/ids';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Button } from '@theme/components/ui/button';
import { Toggle } from '@theme/components/ui/toggle';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useDebounceFn } from '@vueuse/core';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { useLogFilter } from '@workbench/behaviour/logFilter';
import { TIMING } from '@workbench/behaviour/timing';
import CopyLogButton from '@workbench/components/CopyLogButton.vue';
import LogLines from '@workbench/components/LogLines.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { appendCapped, tailItems } from '@workbench/lib/logTail';
import { computed, onBeforeUnmount, ref, shallowRef, useTemplateRef, watch } from 'vue';
import { useDocker } from '../context';
import type { DockerLogLine } from '../wire';

const props = defineProps<{ containerId: string }>();

interface Line extends DockerLogLine {
  id: number;
}

const { control } = useDocker();

const lines = shallowRef<Line[]>([]);
const follow = ref(true);
const timestamps = ref(false);
const wrap = ref(false);
const TAIL_ITEMS = computed(() => tailItems('docker-logs-tail'));
const tail = ref<number>(1000);
const { filter, needle, shown, status } = useLogFilter(
  () => lines.value,
  () => `docker:logs:${props.containerId}:filter`,
);
const ended = ref(false);
const streamError = ref('');
const logsEl = useTemplateRef<{
  atBottom: () => boolean;
  jumpToLatest: () => void;
}>('logsEl');

let streamId = '';
let opening: Promise<unknown> = Promise.resolve();
let seq = 0;
let off: (() => void) | null = null;

function append(batch: DockerLogLine[]): void {
  lines.value = appendCapped(
    lines.value,
    batch.map((l) => ({ ...l, id: seq++ })),
  );
}

async function close(): Promise<void> {
  off?.();
  off = null;
  const id = streamId;
  streamId = '';
  if (!id) return;
  // LogsClose can reach Go before the in-flight LogsOpen registers the stream.
  await opening.catch(() => {
    // The open failure is reported by `open()`; there is no stream left to close.
  });
  await control.logsClose(id).catch(() => {
    // The stream is gone either way; Go drops it with the window.
  });
}

const openLogs = useAsyncAction(async (request: Parameters<typeof control.logsOpen>[0]) => {
  opening = control.logsOpen(request);
  await opening;
});

async function open(): Promise<void> {
  await close();
  lines.value = [];
  ended.value = false;
  openLogs.reset();
  streamError.value = '';
  const id = `docker-logs:${newUuid()}`;
  streamId = id;
  off = control.onLogs((e) => {
    if (e.streamId !== id) return;
    if (e.lines.length > 0) append(e.lines);
    if (e.ended) {
      ended.value = true;
      streamError.value = e.error ?? '';
    }
  });
  await openLogs.run({
    streamId: id,
    containerId: props.containerId,
    tail: tail.value,
    timestamps: timestamps.value,
    follow: true,
  });
}

const reopen = useDebounceFn(() => void open(), TIMING.search);

watch([() => props.containerId, tail, timestamps], () => void reopen(), { immediate: true });
onBeforeUnmount(() => {
  reopen.cancel?.();
  void close();
});

const errorMessage = computed(() => openLogs.error.value || (ended.value ? streamError.value : ''));

function jumpFollow(): void {
  follow.value = true;
  logsEl.value?.jumpToLatest();
}

function clear(): void {
  lines.value = [];
}

// A copy takes what the filter leaves, the same lines the pane shows.
const loadCopy = async () => ({ chunks: shown.value, truncated: false });
</script>

<template>
  <div class="relative flex h-full flex-col" data-testid="docker-logs">
    <ViewToolbar class="h-auto min-h-bar flex-wrap py-1">
      <Toggle
        v-model="follow"
        size="kira"
        data-testid="docker-logs-follow"
        :aria-label="t('common.logs.follow.label')"
        >{{
          t('common.logs.follow.label')
        }}</Toggle
      >
      <Toggle
        v-model="timestamps"
        size="kira"
        data-testid="docker-logs-timestamps"
        :aria-label="t('common.logs.timestamps.label')"
        >{{
          t('common.logs.timestamps.label')
        }}</Toggle
      >
      <Toggle
        v-model="wrap"
        size="kira"
        data-testid="docker-logs-wrap"
        :aria-label="t('common.logs.wrap.label')"
        >{{
          t('common.logs.wrap.label')
        }}</Toggle
      >
      <SecondaryTabs
        variant="segmented"
        :aria-label="t('common.logs.tail.aria')"
        data-testid="docker-logs-tail"
        :model-value="String(tail)"
        :items="TAIL_ITEMS"
        @update:model-value="(v) => (tail = Number(v))"
      />
      <SearchField
        v-model="filter"
        class="ml-auto w-48"
        :placeholder="t('common.logs.filter.placeholder')"
        data-testid="docker-logs-filter"
      />
      <CopyLogButton
        :load="loadCopy"
        :disabled="shown.length === 0"
        data-testid="docker-logs-copy"
      />
      <Button variant="toolbar" size="kira" data-testid="docker-logs-clear" @click="clear"
        ><CodiconIcon :name="ICON.clearAll" :size="13" />
        {{ t('common.action.clear.label') }}</Button
      >
    </ViewToolbar>
    <LogLines
      ref="logsEl"
      testid="docker-log"
      max-height="none"
      class="rounded-none border-0"
      v-model:follow="follow"
      :lines="shown"
      :wrap="wrap"
      :show-timestamps="timestamps"
      :needle="needle"
      :error="errorMessage || undefined"
    />
    <Button
      v-if="!follow && logsEl && !logsEl.atBottom()"
      variant="toolbar"
      size="kira"
      class="absolute bottom-8 right-4"
      data-testid="docker-logs-jump"
      @click="jumpFollow"
      ><CodiconIcon name="arrow-down" :size="13" />
      {{ t('docker.logs.jumpToLatest.label') }}
    </Button>
    <div
      v-if="status || (ended && !errorMessage)"
      class="flex shrink-0 items-center gap-2 bg-chrome px-2 py-0.5 text-kira-sm text-muted-foreground"
    >
      <span v-if="status" data-testid="docker-logs-matches">{{ status }}</span>
      <span v-if="ended && !errorMessage" data-testid="docker-logs-ended">{{
        t('docker.logs.stopped.status')
      }}</span>
    </div>
  </div>
</template>
