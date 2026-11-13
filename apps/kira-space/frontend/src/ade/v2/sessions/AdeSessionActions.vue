<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import AdeTip from '../AdeTip.vue';
import { useStopRun } from '../queries';
import { useAdeOpsStore } from '../state/adeOps';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
import { ACTION_CLASS } from '../tones';
import type { SessionView } from './sessionView';

// Take over and Stop of a background run, at the right end of the session tab row.
const props = defineProps<{ view: SessionView }>();

const ops = useAdeOpsStore();
const takeOver = useAdeTakeOverStore();
const stop = useStopRun();

const stopId = computed(() => `stopRuns:${props.view.session.id}`);

async function onStop(): Promise<void> {
  const { id, taskId, runId } = props.view.session;
  await ops.run(
    {
      id: stopId.value,
      kind: 'stopRuns',
      sessionId: id,
      taskId: taskId || undefined,
      name: props.view.tabName,
    },
    () => stop.mutateAsync({ runId }),
  );
}
</script>

<template>
  <template v-if="view.headless">
    <AdeTip :text="t('space.ade.headless.takeOver.tooltip')">
      <Button
        variant="dialog"
        size="kira"
        class="shrink-0 rounded-kira-sm px-2"
        :class="ACTION_CLASS.agent"
        :pending="takeOver.pending.has(view.session.id)"
        data-testid="ade-session-takeover"
        @click="takeOver.request(view.session.id)"
      >
        {{ t('common.action.takeOver.label') }}
      </Button>
    </AdeTip>
    <AdeTip v-if="view.run?.state === 'running'" :text="t('space.ade.headless.stop.tooltip')">
      <Button
        variant="dialog"
        size="kira"
        class="shrink-0 rounded-kira-sm px-2"
        :pending="ops.isRunning(stopId)"
        data-testid="ade-session-stop"
        @click="onStop"
      >
        {{ t('common.action.stop.label') }}
      </Button>
    </AdeTip>
  </template>
</template>
