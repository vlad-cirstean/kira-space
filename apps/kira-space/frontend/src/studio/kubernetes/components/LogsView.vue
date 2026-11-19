<script setup lang="ts">
import { newUuid } from '@shared/ids';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Button } from '@theme/components/ui/button';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Toggle } from '@theme/components/ui/toggle';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { useLogFilter } from '@workbench/behaviour/logFilter';
import { useKeptState } from '@workbench/behaviour/viewState';
import CopyLogButton from '@workbench/components/CopyLogButton.vue';
import LogLines from '@workbench/components/LogLines.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { appendCapped, tailItems } from '@workbench/lib/logTail';
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import { kubeControl } from '../control';
import type { KubeContainerRef, KubeLogLine, KubeLogsOpen } from '../wire';

const props = defineProps<{
  clusterId: string;
  namespace: string;
  pod: string;
  containers: readonly KubeContainerRef[];
}>();

interface Line extends KubeLogLine {
  id: number;
  stream: string;
}

const logKey = (field: string): string =>
  `kubernetes:logs:${props.clusterId}/${props.namespace}/${props.pod}:${field}`;
const container = useKeptState(() => logKey('container'), '');
const previous = ref(false);
const TAIL_ITEMS = computed(() => tailItems('kube-logs-tail'));
const tail = ref<number>(1000);
const timestamps = ref(false);
const follow = ref(true);
const wrap = ref(false);
const lines = shallowRef<Line[]>([]);
const { filter, needle, shown, status } = useLogFilter(
  () => lines.value,
  () => logKey('filter'),
);
const ended = ref(false);
const streamError = ref('');

let streamId = '';
let opening: Promise<unknown> = Promise.resolve();
let seq = 0;
let off: (() => void) | null = null;

// A copy takes what the filter leaves, the same lines the pane shows.
const loadCopy = async () => ({ chunks: shown.value, truncated: false });

function append(batch: KubeLogLine[]): void {
  lines.value = appendCapped(
    lines.value,
    batch.map((l) => ({ ...l, id: seq++, stream: 'stdout' })),
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
  await kubeControl.logsClose(id).catch(() => {
    // The stream is gone either way; Go drops it with the window.
  });
}

// Bumped by every open() and by unmount; an await that resumes on a stale value bails out.
let generation = 0;

const openStream = useAsyncAction(async (gen: number, request: KubeLogsOpen) => {
  try {
    opening = kubeControl.logsOpen(request);
    await opening;
  } catch (err) {
    if (gen !== generation) return;
    ended.value = true;
    throw err;
  }
});
const failure = computed(() => openStream.error.value || streamError.value);

async function open(): Promise<void> {
  const gen = ++generation;
  await close();
  if (gen !== generation) return;
  lines.value = [];
  ended.value = false;
  streamError.value = '';
  openStream.reset();
  const id = `kube-logs:${newUuid()}`;
  streamId = id;
  off = kubeControl.onLogs((e) => {
    if (e.streamId !== id) return;
    if (e.lines.length > 0) append(e.lines);
    if (e.ended) {
      ended.value = true;
      if (e.error) streamError.value = e.error;
    }
  });
  await openStream.run(gen, {
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
}

watch(
  () => [props.clusterId, props.namespace, props.pod],
  () => {
    if (props.containers.some((c) => c.name === container.value)) return;
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
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="kube-logs">
    <ViewToolbar class="h-auto min-h-bar flex-wrap py-1">
      <NativeSelect
        v-model="container"
        size="kira"
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
      <Toggle v-model="follow" size="kira" data-testid="kube-logs-follow">{{
        t('common.logs.follow.label')
      }}</Toggle>
      <Toggle v-model="timestamps" size="kira" data-testid="kube-logs-timestamps">{{
        t('common.logs.timestamps.label')
      }}</Toggle>
      <Toggle v-model="wrap" size="kira" data-testid="kube-logs-wrap">{{
        t('common.logs.wrap.label')
      }}</Toggle>
      <SecondaryTabs
        variant="segmented"
        :aria-label="t('common.logs.tail.aria')"
        data-testid="kube-logs-tail"
        :model-value="String(tail)"
        :items="TAIL_ITEMS"
        @update:model-value="(v) => (tail = Number(v))"
      />
      <SearchField
        v-model="filter"
        class="ml-auto w-48"
        :placeholder="t('common.logs.filter.placeholder')"
        data-testid="kube-logs-filter"
      />
      <CopyLogButton :load="loadCopy" :disabled="shown.length === 0" data-testid="kube-logs-copy" />
    </ViewToolbar>
    <LogLines
      v-model:follow="follow"
      testid="kube-logs-view"
      max-height="none"
      class="rounded-none border-0"
      :lines="shown"
      :wrap="wrap"
      :show-timestamps="timestamps"
      :needle="needle"
      :error="failure || undefined"
    />
    <div
      class="flex shrink-0 items-center gap-2 bg-chrome px-2 py-0.5 text-kira-sm text-muted-foreground"
    >
      <span v-if="status" data-testid="kube-logs-matches">{{ status }}</span>
      <span v-if="ended" data-testid="kube-logs-ended">{{
        t('kubernetes.logs.ended.status')
      }}</span>
      <Button
        v-if="ended"
        variant="toolbar"
        size="kira"
        data-testid="kube-logs-reconnect"
        @click="open"
        ><CodiconIcon name="plug" :size="13" />{{ t('studio.common.reconnect.label') }}</Button
      >
    </div>
  </div>
</template>
