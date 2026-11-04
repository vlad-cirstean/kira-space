<script setup lang="ts">
import { t } from '@theme/i18n';
import AdeRunLog from '../panel/AdeRunLog.vue';
import type { SessionView } from './sessionView';

// A running background run: its read-only log; Take over and Stop sit in the tab row.
defineProps<{ view: SessionView }>();
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" data-testid="ade-headless-pane">
    <div class="min-h-0 flex-1 overflow-auto p-2">
      <AdeRunLog v-if="view.session.runId" kind="run" :id="view.session.runId" max-height="100%" />
    </div>
    <div class="shrink-0 px-3 py-2 text-kira-sm text-subtle">
      {{ t('space.ade.headless.footer.hint') }}
    </div>
  </div>
</template>
