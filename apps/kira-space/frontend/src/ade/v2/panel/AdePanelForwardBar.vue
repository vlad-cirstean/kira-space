<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@theme/components/ui/collapsible';
import { TooltipDisabledTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref } from 'vue';
import AdeTip from '../AdeTip.vue';
import { useDismissPanelEdits, useForwardPanelEdits, usePanelForward } from '../queries';

// One offer per task for the panel edits not yet sent to the agents running in its step.
const props = defineProps<{ taskId: string }>();

const offer = usePanelForward(() => props.taskId);
const forwardM = useForwardPanelEdits();
const dismissM = useDismissPanelEdits();
const open = ref(false);
const error = ref('');
const failed = ref('');

const data = computed(() => offer.data.value);
const shown = computed(() => (data.value?.items.length ?? 0) > 0);
const badgeOf = (sessionId: string) => data.value?.targets.find((x) => x.sessionId === sessionId);
const noTarget = computed(() => (data.value?.targets.length ?? 0) === 0);
const off = computed(() => noTarget.value || forwardM.isPending.value);

async function forward(): Promise<void> {
  error.value = '';
  failed.value = '';
  try {
    const res = await forwardM.mutateAsync({ taskId: props.taskId });
    if (res.failed.length > 0) {
      failed.value = res.failed
        .map((f) => `${badgeOf(f.sessionId)?.badge ?? f.sessionId}: ${f.error}`)
        .join('; ');
    }
  } catch (err) {
    error.value = errorText(err);
  }
}

async function dismiss(): Promise<void> {
  error.value = '';
  failed.value = '';
  try {
    await dismissM.mutateAsync({ taskId: props.taskId });
  } catch (err) {
    error.value = errorText(err);
  }
}
</script>

<template>
  <div v-if="shown && data" class="flex flex-col gap-1" data-testid="ade-panel-forward-bar">
    <Alert variant="info" strip>
      <AlertDescription>
        <span class="flex flex-col gap-1.5"
          ><span class="flex items-center gap-2">
            <CodiconIcon name="send" :size="13" />
            <span class="min-w-0 flex-1" data-testid="ade-panel-forward-text">{{
              t('space.ade.panelForward.pending.body', {
                count: data.items.length,
                step: data.stepName,
              })
            }}</span>
            <AdeTip :text="noTarget ? t('space.ade.panelForward.unreachable.tooltip') : ''">
              <TooltipDisabledTrigger :disabled="off">
                <Button
                  variant="dialog"
                  size="kira"
                  :pending="forwardM.isPending.value"
                  :disabled="off"
                  data-testid="ade-panel-forward"
                  @click="forward"
                >
                  {{ t('space.ade.panelForward.send.label', { count: data.targets.length }) }}
                </Button>
              </TooltipDisabledTrigger>
            </AdeTip>
            <TooltipIconButton
              icon="close"
              :label="t('space.ade.panelForward.discard.tooltip')"
              :disabled="dismissM.isPending.value"
              data-testid="ade-panel-forward-dismiss"
              @click="dismiss"
            />
          </span>
          <span class="flex flex-wrap gap-1" data-testid="ade-panel-forward-items">
            <Badge v-for="it in data.items" :key="it.key">{{ it.label }}</Badge>
          </span>
          <span v-if="data.dropped > 0" data-testid="ade-panel-forward-dropped">{{
            t('space.ade.panelForward.dropped.body')
          }}</span>
          <Collapsible v-model:open="open">
            <CollapsibleTrigger
              class="flex items-center gap-1 text-kira-sm"
              data-testid="ade-panel-forward-toggle"
            >
              <CodiconIcon :name="open ? 'chevron-down' : 'chevron-right'" :size="12" />
              {{ t('space.ade.panelForward.preview.label') }}
            </CollapsibleTrigger>
            <CollapsibleContent>
              <pre
                class="max-h-60 overflow-auto whitespace-pre-wrap rounded-kira-sm p-2 font-mono text-kira-sm"
                data-testid="ade-panel-forward-preview"
              >{{ data.preview }}</pre>
            </CollapsibleContent>
          </Collapsible></span
        >
      </AlertDescription>
    </Alert>
  </div>
  <Alert v-if="failed" variant="error" data-testid="ade-panel-forward-failed"
    ><AlertDescription>{{
      t('space.ade.panelForward.failed.body', { sessions: failed })
    }}</AlertDescription></Alert
  >
  <Alert v-if="error" variant="error" data-testid="ade-panel-forward-error"
    ><AlertDescription>{{ error }}</AlertDescription></Alert
  >
</template>
