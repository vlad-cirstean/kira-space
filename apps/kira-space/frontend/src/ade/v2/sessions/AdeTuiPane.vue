<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { toast } from '@theme/components/ui/sonner';
import { t } from '@theme/i18n';
import { useDateFormat } from '@vueuse/core';
import { useAutomationsModule } from '@workbench/automations/module';
import { errorText } from '@workbench/bridge/codedError';
import TerminalHostView from '@workbench/terminal/TerminalHostView.vue';
import { computed, ref, useTemplateRef } from 'vue';
import { useMobileTerminalsStore } from '../../../state/mobileTerminals';
import { useTerminalsStore } from '../../../state/terminals';
import AdeRunSandboxBadge from '../panel/AdeRunSandboxBadge.vue';
import { useFocusSession } from '../queries';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import type { SessionView } from './sessionView';
import { useMarkSeenOnView } from './useMarkSessionSeen';

// An interactive session: the terminal when this window holds it, else a Show button that raises
// the window that does.
const props = defineProps<{ view: SessionView }>();

const terminals = useTerminalsStore();
const phones = useMobileTerminalsStore();
const terminal = useAutomationsModule();
const focus = useFocusSession();
const tabs = useAdePanelTabsStore();
const missing = ref(false);
const host = useTemplateRef<InstanceType<typeof TerminalHostView>>('host');

const held = computed(() => terminals.terminalSession(props.view.session.terminalId) !== undefined);
const hold = computed(() => phones.holdOf(props.view.session.terminalId));
const returnsAt = useDateFormat(() => hold.value?.returnsAt ?? 0, 'HH:mm');
const tab = computed(() => ({
  id: props.view.session.terminalId,
  state: {
    codeRepoId: '',
    cwd: props.view.session.cwd,
    command: '',
    launchKind: 'agent' as const,
  },
}));

useMarkSeenOnView(
  () => props.view.session.id,
  () => props.view.session.unseen,
  held,
);

// Gives the keyboard to the terminal; nothing to focus while a phone holds it or another window does.
function focusTerminal(): void {
  if (!held.value || hold.value) return;
  host.value?.focusTerminal();
}
defineExpose({ focusTerminal });

async function show(): Promise<void> {
  missing.value = false;
  let failed = false;
  const shown = await focus
    .mutateAsync({ sessionId: props.view.session.id, taskId: props.view.session.taskId })
    .catch((err) => {
      failed = true;
      toast.error(errorText(err));
      return false;
    });
  if (failed) return;
  if (!shown) missing.value = true;
  else tabs.selectSession(props.view.session.taskId, props.view.session.id);
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" data-testid="ade-tui-pane">
    <div v-if="view.session.sandbox" class="flex shrink-0 items-center bg-chrome px-1.5">
      <AdeRunSandboxBadge :session-id="view.session.id" :badge="view.session.sandbox" />
    </div>
    <div v-if="held" class="relative flex min-h-0 flex-1 flex-col">
      <TerminalHostView
        ref="host"
        :key="view.session.terminalId"
        class="min-h-0 flex-1"
        :tab="tab"
        :deps="terminal.host"
      />
      <div
        v-if="hold"
        class="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-bg p-4 text-center text-kira-md"
        data-testid="ade-tui-phone-overlay"
      >
        <p class="m-0">{{ t('space.ade.tui.controlled.label', { device: hold.label }) }}</p>
        <p
          v-if="!hold.connected"
          class="m-0 text-muted-foreground"
          data-testid="ade-tui-phone-offline"
        >
          {{ t('space.ade.tui.offline.status', { time: returnsAt }) }}
        </p>
        <Button
          variant="dialog"
          size="kira"
          class="px-2"
          data-testid="ade-tui-reconnect"
          @click="phones.reclaim(view.session.terminalId)"
        >
          {{ t('space.ade.tui.reconnect.label') }}
        </Button>
      </div>
    </div>
    <div
      v-else
      class="flex flex-col items-start gap-2 p-3 text-kira-md"
      data-testid="ade-tui-elsewhere"
    >
      <Button
        variant="dialog"
        size="kira"
        class="px-2"
        data-testid="ade-tui-show"
        :pending="focus.isPending.value"
        @click="show"
      >
        {{ t('common.action.show.label') }}
      </Button>
      <p v-if="missing" class="m-0 text-muted-foreground" data-testid="ade-tui-missing">
        {{ t('space.ade.tui.missing.hint') }}
      </p>
    </div>
  </div>
</template>
