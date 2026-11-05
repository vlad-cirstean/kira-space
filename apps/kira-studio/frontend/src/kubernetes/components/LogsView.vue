<script setup lang="ts">
import { newUuid } from '@shared/ids';
import HighlightedText from '@theme/components/HighlightedText.vue';
import SearchField from '@theme/components/SearchField.vue';
import { Button } from '@theme/components/ui/button';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Toggle } from '@theme/components/ui/toggle';
import { t } from '@theme/i18n';
import { matchesQuery, splitMatches } from '@workbench/behaviour/match';
import { errorText } from '@workbench/bridge/codedError';
import LogLines from '@workbench/components/LogLines.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { type AnsiSegment, parseAnsi } from '@workbench/lib/ansi';
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import { kubeControl } from '../control';
import type { KubeContainerRef, KubeLogLine } from '../wire';

const props = defineProps<{
  clusterId: string;
  namespace: string;
  pod: string;
  containers: readonly KubeContainerRef[];
}>();

const MAX_LINES = 20_000;
const TAILS = [100, 1000, 5000, -1] as const;
const tailLabel = (n: number): string =>
  n === -1 ? t('common.state.all.option') : t('docker.logs.tail.option', { count: n });

interface Line extends KubeLogLine {
  id: number;
  stream: string;
  plain: string;
}

const container = ref('');
const previous = ref(false);
const tail = ref<number>(1000);
const timestamps = ref(false);
const follow = ref(true);
const wrap = ref(false);
const filter = ref('');
const lines = shallowRef<Line[]>([]);
const ended = ref(false);
const failure = ref('');

let streamId = '';
let opening: Promise<unknown> = Promise.resolve();
let seq = 0;
let off: (() => void) | null = null;

const plainOf = (text: string): string =>
  parseAnsi(text)
    .map((seg) => seg.text)
    .join('');

const needle = computed(() => filter.value.trim().toLowerCase());
const shown = computed(() =>
  needle.value === ''
    ? lines.value
    : lines.value.filter((l) => matchesQuery(l.plain, needle.value)),
);

function append(batch: KubeLogLine[]): void {
  const next = lines.value.concat(
    batch.map((l) => ({ ...l, id: seq++, stream: 'stdout', plain: plainOf(l.text) })),
  );
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
  await kubeControl.logsClose(id).catch(() => {
    // The stream is gone either way; Go drops it with the window.
  });
}

// Bumped by every open() and by unmount; an await that resumes on a stale value bails out.
let generation = 0;

async function open(): Promise<void> {
  const gen = ++generation;
  await close();
  if (gen !== generation) return;
  lines.value = [];
  ended.value = false;
  failure.value = '';
  const id = `kube-logs:${newUuid()}`;
  streamId = id;
  off = kubeControl.onLogs((e) => {
    if (e.streamId !== id) return;
    if (e.lines.length > 0) append(e.lines);
    if (e.ended) {
      ended.value = true;
      if (e.error) failure.value = e.error;
    }
  });
  try {
    opening = kubeControl.logsOpen({
      streamId: id,
      clusterId: props.clusterId,
      namespace: props.namespace,
      pod: props.pod,
      container: container.value,
      previous: previous.value,
      tail: tail.value,
      timestamps: timestamps.value,
      follow: !previous.value,
    });
    await opening;
  } catch (err) {
    if (gen !== generation) return;
    failure.value = errorText(err);
    ended.value = true;
  }
}

watch(
  () => [props.clusterId, props.namespace, props.pod],
  () => {
    container.value =
      props.containers.find((c) => !c.init)?.name ?? props.containers[0]?.name ?? '';
  },
  { immediate: true },
);
watch(
  [
    () => props.clusterId,
    () => props.namespace,
    () => props.pod,
    container,
    previous,
    tail,
    timestamps,
  ],
  () => void open(),
  { immediate: true },
);
onBeforeUnmount(() => {
  generation++;
  void close();
});

function segmentStyle(seg: AnsiSegment): Record<string, string> | undefined {
  if (!seg.color && !seg.background) return undefined;
  return {
    ...(seg.color && { color: seg.color }),
    ...(seg.background && { backgroundColor: seg.background }),
  };
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="kube-logs">
    <ViewToolbar class="flex-wrap">
      <NativeSelect
        v-model="container"
        :aria-label="t('kubernetes.logs.container.label')"
        data-testid="kube-logs-container"
      >
        <option v-for="c in containers" :key="c.name" :value="c.name">{{
          c.init ? t('kubernetes.logs.initContainer.option', { name: c.name }) : c.name
        }}</option>
      </NativeSelect>
      <Toggle v-model="previous" size="kira" data-testid="kube-logs-previous">{{
        t('common.action.previous.label')
      }}</Toggle>
      <NativeSelect
        v-model="tail"
        :aria-label="t('docker.logs.tail.aria')"
        data-testid="kube-logs-tail"
      >
        <option v-for="n in TAILS" :key="n" :value="n">{{ tailLabel(n) }}</option>
      </NativeSelect>
      <Toggle v-model="timestamps" size="kira" data-testid="kube-logs-timestamps">{{
        t('docker.logs.timestamps.label')
      }}</Toggle>
      <Toggle v-model="follow" size="kira" data-testid="kube-logs-follow">{{
        t('docker.logs.follow.label')
      }}</Toggle>
      <Toggle v-model="wrap" size="kira" data-testid="kube-logs-wrap">{{
        t('docker.logs.wrap.label')
      }}</Toggle>
      <SearchField
        v-model="filter"
        class="ml-auto w-48"
        :placeholder="t('kubernetes.logs.filter.placeholder')"
        data-testid="kube-logs-filter"
      />
    </ViewToolbar>
    <LogLines
      v-model:follow="follow"
      testid="kube-logs-view"
      max-height="none"
      class="rounded-none border-0"
      :lines="shown"
      :wrap="wrap"
      :error="failure || undefined"
    >
      <template #line="{ segments, index }">
        <span v-if="timestamps && shown[index]?.ts" class="mr-2 text-muted-foreground">{{
          shown[index]?.ts
        }}</span>
        <span
          v-for="(seg, i) in segments"
          :key="i"
          :class="seg.bold && 'font-medium'"
          :style="segmentStyle(seg)"
          ><HighlightedText :parts="splitMatches(seg.text, needle)" /></span
        >
      </template>
    </LogLines>
    <div
      class="flex shrink-0 items-center gap-2 bg-chrome px-2 py-0.5 text-kira-sm text-muted-foreground"
    >
      <span v-if="needle !== ''" data-testid="kube-logs-matches">{{
        t('kubernetes.logs.matches.status', { shown: shown.length, total: lines.length })
      }}</span>
      <span v-if="ended" data-testid="kube-logs-ended">{{
        t('kubernetes.logs.ended.status')
      }}</span>
      <Button
        v-if="ended"
        variant="toolbar"
        size="kira"
        data-testid="kube-logs-reconnect"
        @click="open"
        >{{
          t('studio.common.reconnect.label')
        }}</Button
      >
    </div>
  </div>
</template>
