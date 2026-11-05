<script setup lang="ts">
import { newUuid } from '@shared/ids';
import HighlightedText from '@theme/components/HighlightedText.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Toggle } from '@theme/components/ui/toggle';
import { t } from '@theme/i18n';
import { useDebounceFn } from '@vueuse/core';
import { matchesQuery, splitMatches } from '@workbench/behaviour/match';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import FindBar from '@workbench/components/FindBar.vue';
import LogLines from '@workbench/components/LogLines.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { type AnsiSegment, parseAnsi } from '@workbench/lib/ansi';
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, useTemplateRef, watch } from 'vue';
import { useDocker } from '../context';
import type { DockerLogLine } from '../wire';

const props = defineProps<{ containerId: string }>();

const MAX_LINES = 20_000;
const TAILS = [100, 1000, 5000, -1] as const;

const tailLabel = (n: number): string =>
  n === -1 ? t('common.state.all.option') : t('docker.logs.tail.option', { count: n });

interface Line extends DockerLogLine {
  id: number;
  /** ANSI-stripped text, filled on first find scan so later batches parse only new lines. */
  plain?: string;
}

const { control } = useDocker();

const lines = shallowRef<Line[]>([]);
const follow = ref(true);
const timestamps = ref(false);
const wrap = ref(false);
const tail = ref<number>(1000);
const findOpen = ref(false);
const query = ref('');
const needle = ref('');
const current = ref(0);
const ended = ref(false);
const failure = ref('');
const streamError = ref('');
const logsEl = useTemplateRef<{
  scrollToIndex: (index: number) => void;
  atBottom: () => boolean;
  jumpToLatest: () => void;
}>('logsEl');

let streamId = '';
let opening: Promise<unknown> = Promise.resolve();
let seq = 0;
let off: (() => void) | null = null;

const matchLines = computed<number[]>(() => {
  const q = needle.value;
  if (q === '') return [];
  const out: number[] = [];
  lines.value.forEach((l, i) => {
    l.plain ??= plain(l.text);
    if (matchesQuery(l.plain, q)) out.push(i);
  });
  return out;
});
const currentLine = computed(() => matchLines.value[current.value]);

const plain = (text: string): string =>
  parseAnsi(text)
    .map((seg) => seg.text)
    .join('');

function onQuery(q: string): void {
  needle.value = q.trim().toLowerCase();
  current.value = 0;
  void nextTick(revealCurrent);
}

function step(delta: number): void {
  const n = matchLines.value.length;
  if (n === 0) return;
  current.value = (current.value + delta + n) % n;
  void nextTick(revealCurrent);
}

function closeFind(): void {
  findOpen.value = false;
  query.value = '';
  needle.value = '';
}

function revealCurrent(): void {
  const index = currentLine.value;
  if (index === undefined) return;
  follow.value = false;
  logsEl.value?.scrollToIndex(index);
}

watch(matchLines, (m) => {
  if (current.value >= m.length) current.value = Math.max(0, m.length - 1);
});

function append(batch: DockerLogLine[]): void {
  const next = lines.value.concat(batch.map((l) => ({ ...l, id: seq++ })));
  lines.value = next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next;
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

async function open(): Promise<void> {
  await close();
  lines.value = [];
  ended.value = false;
  failure.value = '';
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
  try {
    opening = control.logsOpen({
      streamId: id,
      containerId: props.containerId,
      tail: tail.value,
      timestamps: timestamps.value,
      follow: true,
    });
    await opening;
  } catch (err) {
    failure.value = errorText(err);
  }
}

const reopen = useDebounceFn(() => void open(), TIMING.search);

watch([() => props.containerId, tail, timestamps], () => void reopen(), { immediate: true });
onBeforeUnmount(() => {
  reopen.cancel?.();
  void close();
});

const errorMessage = computed(() => failure.value || (ended.value ? streamError.value : ''));

function segmentStyle(seg: AnsiSegment): Record<string, string> | undefined {
  if (!seg.color && !seg.background) return undefined;
  return {
    ...(seg.color && { color: seg.color }),
    ...(seg.background && { backgroundColor: seg.background }),
  };
}

function jumpFollow(): void {
  follow.value = true;
  logsEl.value?.jumpToLatest();
}

function clear(): void {
  lines.value = [];
}
</script>

<template>
  <div class="relative flex h-full flex-col" data-testid="docker-logs">
    <ViewToolbar class="flex-wrap">
      <Toggle
        v-model="follow"
        size="kira"
        data-testid="docker-logs-follow"
        :aria-label="t('docker.logs.follow.label')"
        >{{
          t('docker.logs.follow.label')
        }}</Toggle
      >
      <Toggle
        v-model="timestamps"
        size="kira"
        data-testid="docker-logs-timestamps"
        :aria-label="t('docker.logs.timestamps.label')"
        >{{
          t('docker.logs.timestamps.label')
        }}</Toggle
      >
      <Toggle
        v-model="wrap"
        size="kira"
        data-testid="docker-logs-wrap"
        :aria-label="t('docker.logs.wrap.label')"
        >{{
          t('docker.logs.wrap.label')
        }}</Toggle
      >
      <NativeSelect
        v-model="tail"
        :aria-label="t('docker.logs.tail.aria')"
        data-testid="docker-logs-tail"
      >
        <option v-for="n in TAILS" :key="n" :value="n">{{ tailLabel(n) }}</option>
      </NativeSelect>
      <TooltipIconButton
        class="ml-auto"
        icon="search"
        :label="t('common.action.find.label')"
        :pressed="findOpen"
        data-testid="docker-logs-find"
        @click="findOpen = !findOpen"
      />
      <Button variant="toolbar" size="kira" data-testid="docker-logs-clear" @click="clear">{{
        t('common.action.clear.label')
      }}</Button>
    </ViewToolbar>
    <FindBar
      v-if="findOpen"
      v-model="query"
      testid-prefix="docker-logs-find-"
      :position="matchLines.length === 0 ? 0 : current + 1"
      :total="matchLines.length"
      @query="onQuery"
      @next="step(1)"
      @prev="step(-1)"
      @close="closeFind"
    />
    <LogLines
      ref="logsEl"
      testid="docker-log"
      max-height="none"
      class="rounded-none border-0"
      v-model:follow="follow"
      :lines="lines"
      :wrap="wrap"
      :error="errorMessage || undefined"
    >
      <template #line="{ segments, index }">
        <span v-if="timestamps && lines[index]?.ts" class="mr-2 text-muted-foreground">{{
          lines[index]?.ts
        }}</span>
        <span
          v-for="(seg, i) in segments"
          :key="i"
          :class="seg.bold && 'font-bold'"
          :style="segmentStyle(seg)"
          ><HighlightedText
            :parts="splitMatches(seg.text, needle)"
            :current="
              index === currentLine
                ? splitMatches(seg.text, needle).findIndex((p) => p.match)
                : undefined
            "
          /></span
        >
      </template>
    </LogLines>
    <Button
      v-if="!follow && logsEl && !logsEl.atBottom()"
      variant="toolbar"
      size="kira"
      class="absolute bottom-8 right-4"
      data-testid="docker-logs-jump"
      @click="jumpFollow"
    >
      {{ t('docker.logs.jumpToLatest.label') }}
    </Button>
    <div
      v-if="ended && !errorMessage"
      class="shrink-0 bg-chrome px-2 py-0.5 text-kira-sm text-muted-foreground"
      data-testid="docker-logs-ended"
    >
      {{ t('docker.logs.stopped.status') }}
    </div>
  </div>
</template>
