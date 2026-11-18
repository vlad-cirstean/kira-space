<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useStopRun } from '../queries';
import { useAdeOpsStore } from '../state/adeOps';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
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
    <TextTooltip :text="t('space.ade.headless.takeOver.tooltip')">
      <Button
        variant="toolbar"
        size="kira"
        class="shrink-0"
        :pending="takeOver.pending.has(view.session.id)"
        data-testid="ade-session-takeover"
        @click="takeOver.request(view.session.id)"
      >
        <CodiconIcon name="terminal" :size="13" />{{ t('common.action.takeOver.label') }}
      </Button>
    </TextTooltip>
    <TextTooltip v-if="view.run?.state === 'running'" :text="t('space.ade.headless.stop.tooltip')">
      <Button
        variant="toolbar"
        size="kira"
        class="shrink-0"
        data-tone="error"
        :pending="ops.isRunning(stopId)"
        data-testid="ade-session-stop"
        @click="onStop"
      >
        <CodiconIcon name="debug-stop" :size="13" />{{ t('common.action.stop.label') }}
      </Button>
    </TextTooltip>
  </template>
</template>
