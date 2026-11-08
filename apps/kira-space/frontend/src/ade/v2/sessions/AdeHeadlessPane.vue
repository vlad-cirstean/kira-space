<script setup lang="ts">
import { t } from '@theme/i18n';
import CopyLogButton from '@workbench/components/CopyLogButton.vue';
import AdeRunLog from '../panel/AdeRunLog.vue';
import { useLog } from '../readQueries';
import type { SessionView } from './sessionView';

// A running background run: its read-only log; Take over and Stop sit in the tab row.
const props = defineProps<{ view: SessionView }>();

// Same query key as AdeRunLog: one fetch.
const log = useLog(
  () => 'run',
  () => props.view.session.runId ?? '',
);
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
        :chunks="log.data.value?.chunks ?? []"
        data-testid="ade-headless-copy-log"
      />
    </div>
  </div>
</template>
