<script setup lang="ts">
import { pathTail } from '@shared/domain/tree';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { colorMarkClass, connTextClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { registerCommand } from '@workbench/shortcuts/commands';
import { computed, onMounted, onUnmounted } from 'vue';
import type { StreamTabRecord } from '../../state/tabDomain';
import EngineIcon from '../../theme/EngineIcon.vue';
import CellEditorDock from '../shared/celleditor/CellEditorDock.vue';
import StaleDataStrip from '../shared/StaleDataStrip.vue';
import { refreshOrReconnect, useConnectionGate } from '../shared/useConnectionGate';
import StreamFilterRow from './StreamFilterRow.vue';
import StreamRowsTable from './StreamRowsTable.vue';
import StreamToolbar from './StreamToolbar.vue';
import { useStreamViewStore } from './state';
import { redriveSentence, useStreamGate } from './useStreamGate';

const streamViewStore = useStreamViewStore();

// MainView.vue keys this component by tab.id.
const props = defineProps<{ tab: StreamTabRecord }>();

const gate = useStreamGate(() => props.tab);
const { connRecord, rt, page, isBatch, isKafka, isReadOnlyBatch, redriveLimit, hideDuration } =
  gate;

const iconClass = computed(() => connTextClass(connRecord.value?.color) ?? 'text-muted-foreground');
const pathPrefix = computed(() => (connRecord.value ? `${connRecord.value.name} / ` : ''));
const railColor = computed(() => (connRecord.value ? (connRecord.value.color ?? null) : undefined));
const targetTail = computed(() => pathTail(props.tab.path));

// A batch tab (SQS) never auto-loads on reconnect: only an explicit Poll reads.
const { needsReconnect, reconnecting, onReconnectAndLoad } = useConnectionGate(
  () => props.tab,
  () => {
    if (!isBatch.value) return streamViewStore.load(props.tab.id);
  },
);

// Inert for SQS: view.refresh reaches this directly, and a refresh must never call ReceiveMessage.
function onRefresh(): void {
  if (isBatch.value) return;
  refreshOrReconnect(needsReconnect.value, onReconnectAndLoad, () =>
    streamViewStore.reload(props.tab.id),
  );
}

let unregisterCommand: (() => void) | null = null;
let unregisterFindCommand: (() => void) | null = null;

onMounted(() => {
  if (!needsReconnect.value && !isBatch.value && !streamViewStore.runtime[props.tab.id]) {
    void streamViewStore.load(props.tab.id);
  }
  unregisterCommand = registerCommand('view.refresh', onRefresh);
  unregisterFindCommand = registerCommand('view.find', () =>
    streamViewStore.toggleSearchOpen(props.tab.id),
  );
});

onUnmounted(() => {
  unregisterCommand?.();
  unregisterFindCommand?.();
});
</script>

<template>
  <div class="h-full flex flex-col min-h-0" data-testid="stream-view" :data-path="tab.path">
    <div class="flex flex-1 min-h-0 flex-col">
      <div class="flex min-h-0 flex-1 flex-col">
        <ViewToolbar>
          <span v-if="railColor !== undefined" :class="colorMarkClass('dot', railColor)" />
          <span v-if="connRecord?.kind" class="size-4 flex items-center justify-center shrink-0">
            <EngineIcon :kind="connRecord.kind" :size="13" />
          </span>
          <span class="size-4 flex items-center justify-center shrink-0" :class="iconClass">
            <CodiconIcon name="broadcast" :size="13" />
          </span>
          <span class="text-kira-md text-fg truncate"
            ><span v-if="pathPrefix" class="text-subtle">{{ pathPrefix }}</span
            ><span data-testid="stream-target">{{ targetTail?.name ?? tab.path }}</span></span
          >
          <span class="ml-auto flex items-center gap-1">
            <Badge
              v-if="
                page?.visibilityTimeoutSeconds !== null &&
                page?.visibilityTimeoutSeconds !== undefined
              "
              data-testid="stream-visibility-timeout"
            >
              {{
                t(
                  isReadOnlyBatch
                    ? 'studio.stream.visibilityQueue.status'
                    : 'studio.stream.visibility.status',
                  {
                    time: t('format.duration.secondsCompact', {
                      seconds: page.visibilityTimeoutSeconds,
                    }),
                  },
                )
              }}
            </Badge>
          </span>
        </ViewToolbar>

        <div :class="colorMarkClass('band', railColor)" />
        <StreamToolbar :tab="tab" @refresh="onRefresh" />
        <StreamFilterRow v-if="isKafka" :tab="tab" />

        <StaleDataStrip
          v-if="gate.stale.value && page"
          testid="stream-stale"
          :reconnecting="reconnecting"
          @reconnect="onReconnectAndLoad"
        />
        <Alert v-if="isBatch" variant="warn" strip data-testid="stream-poll-warning">
          <AlertDescription v-if="isReadOnlyBatch">
            {{ t('studio.stream.pollWarningReadOnly.description', { duration: hideDuration }) }}
            <template v-if="redriveLimit !== null">{{ redriveSentence(redriveLimit) }}</template>
          </AlertDescription>
          <AlertDescription v-else>
            {{ t('studio.stream.pollWarningConsume.description') }}
            <template v-if="redriveLimit !== null">{{ redriveSentence(redriveLimit) }}</template>
          </AlertDescription>
        </Alert>

        <Alert
          v-if="rt?.status === 'error' && rt.error"
          variant="error"
          strip
          data-testid="stream-error"
        >
          <AlertDescription>{{ rt.error.message }}</AlertDescription>
        </Alert>
        <Alert v-if="rt?.actionError" variant="error" strip data-testid="stream-action-error">
          <AlertDescription>{{ rt.actionError }}</AlertDescription>
        </Alert>

        <EmptyState
          v-if="needsReconnect && !page"
          data-testid="stream-reconnect"
          icon="plug"
          :title="t('studio.common.disconnected.empty')"
        >
          <Button
            variant="dialog-primary"
            size="kira-lg"
            data-testid="stream-reconnect-load"
            :pending="reconnecting"
            @click="onReconnectAndLoad"
          >
            {{ t('studio.common.reconnectAndLoad.label') }}
          </Button>
        </EmptyState>
        <StreamRowsTable v-else :tab="tab" />
      </div>
      <CellEditorDock :tab-id="tab.id" readonly />
    </div>
  </div>
</template>
