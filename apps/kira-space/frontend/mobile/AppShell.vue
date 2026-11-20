<script setup lang="ts">
import { useQueryClient } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import DetailHeader from '@theme/components/DetailHeader.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useOnline } from '@vueuse/core';
import { useAgentMessageDefaults } from '@workbench/agent/agentInfo';
import { onMounted } from 'vue';
import { useServerEvents } from './api/events';
import { useAgentSessionsStore } from './state/agentSessions';
import { phoneKey } from './state/usePhoneStatus';
import TabBar from './TabBar.vue';

const queryClient = useQueryClient();
const agentSessions = useAgentSessionsStore();
const online = useOnline();
useAgentMessageDefaults(() => undefined);

function refresh(): void {
  void queryClient.invalidateQueries({ queryKey: ['adetask'] });
}

// Pushes sent while the stream was down are lost: refetch everything and re-read the agent state.
const { connection } = useServerEvents(() => {
  void queryClient.invalidateQueries({ queryKey: ['adetask'] });
  void queryClient.invalidateQueries({ queryKey: ['approvals'] });
  void queryClient.invalidateQueries({ queryKey: phoneKey });
  void agentSessions.initAgentSessions();
});
onMounted(() => void agentSessions.initAgentSessions());

const DOT: Record<typeof connection.value, 'ok' | 'warn' | 'error'> = {
  live: 'ok',
  reconnecting: 'warn',
  offline: 'error',
};
const DOT_LABEL = {
  live: 'mobile.shell.connection.live.aria',
  reconnecting: 'mobile.shell.connection.reconnecting.aria',
  offline: 'mobile.shell.connection.offline.aria',
} as const;
</script>

<template>
  <div class="flex h-full flex-col pt-[env(safe-area-inset-top)]" data-testid="app-shell">
    <DetailHeader
      as="h1"
      class="border-b border-border bg-chrome"
      :title="t('common.module.agents.label')"
    >
      <template #meta>
        <StatusDot
          :tone="DOT[connection]"
          role="status"
          :label="t(DOT_LABEL[connection])"
          data-testid="connection-dot"
        />
      </template>
      <template #actions>
        <Button
          size="touch-icon"
          variant="toolbar"
          :aria-label="t('common.action.refresh.label')"
          data-testid="refresh"
          @click="refresh"
        >
          <CodiconIcon name="refresh" :size="13" />
        </Button>
      </template>
    </DetailHeader>
    <Alert v-if="!online" variant="warn" strip data-testid="offline-banner">
      <AlertDescription>{{ t('mobile.shell.offline.description') }}</AlertDescription>
    </Alert>
    <main class="min-h-0 flex-1 overflow-auto">
      <RouterView />
    </main>
    <TabBar />
  </div>
</template>
