<script setup lang="ts">
import { t } from '@theme/i18n';
import CopyLogButton from '@workbench/components/CopyLogButton.vue';
import AdeRunLog from '../panel/AdeRunLog.vue';
import { useAdeReader } from '../reader';
import { useLog } from '../readQueries';
import type { LogChunk } from '../wire';
import type { SessionView } from './sessionView';
import { useMarkSeenOnView } from './useMarkSessionSeen';

// A running background run: its read-only log; Take over and Stop sit in the tab row.
const props = defineProps<{ view: SessionView }>();

// Same query key as AdeRunLog: one fetch.
const log = useLog(
  () => 'run',
  () => props.view.session.runId ?? '',
);

useMarkSeenOnView(
  () => props.view.session.id,
  () => props.view.session.unseen,
);

const reader = useAdeReader();
// Mirrors `adeLogPageLimit` in storage/repos/adelogs.go: a shorter page is the last.
const LOG_PAGE_CHUNKS = 500;

// The cached page holds the log's first page and later pushes; the copy reads every page.
async function loadWholeLog(runId: string) {
  const chunks: LogChunk[] = [];
  let truncated = false;
  for (let afterSeq = 0; ; ) {
    const page = await reader.readLog({ kind: 'run', id: runId, afterSeq });
    truncated ||= page.truncated;
    chunks.push(...page.chunks);
    const last = page.chunks.at(-1);
    if (!last || page.chunks.length < LOG_PAGE_CHUNKS) return { chunks, truncated };
    afterSeq = last.seq;
  }
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" data-testid="ade-headless-pane">
    <div class="flex min-h-0 flex-1 flex-col p-2">
      <AdeRunLog v-if="view.session.runId" kind="run" :id="view.session.runId" max-height="none" />
    </div>
    <div class="flex shrink-0 items-center gap-2 px-3 py-2">
      <span class="min-w-0 flex-1 text-kira-sm text-subtle">{{
        t('space.ade.headless.footer.hint')
      }}</span>
      <CopyLogButton
        v-if="view.session.runId"
        :load="() => loadWholeLog(view.session.runId ?? '')"
        :disabled="(log.data.value?.chunks.length ?? 0) === 0"
        data-testid="ade-headless-copy-log"
      />
    </div>
  </div>
</template>
