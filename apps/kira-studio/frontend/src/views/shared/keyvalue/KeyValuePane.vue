<script setup lang="ts">
// With `tab` the pane renders its own chrome and reconnect gate (a main-tab view); without it
// (the browse split's preview pane) only the content renders, since BrowseView gates the whole split.
import { decodePath } from '@shared/domain/tree';
import { SINGLE_ROW_MAX_BYTES } from '@shared/protocol/page';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { colorMarkClass, connTextClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import RunState from '@theme/RunState.vue';
import { formatBytes } from '@workbench/behaviour/format';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { registerCommand } from '@workbench/shortcuts/commands';
import { computed, onMounted, onUnmounted, useTemplateRef, watch } from 'vue';
import { useRunState } from '../../../state/runState';
import type { KeyValueTabRecord } from '../../../state/tabDomain';
import EngineIcon from '../../../theme/EngineIcon.vue';
import CellEditorDock from '../celleditor/CellEditorDock.vue';
import { usePublishEditDirty } from '../editDirty';
import StaleDataStrip from '../StaleDataStrip.vue';
import { refreshOrReconnect, useConnectionGate } from '../useConnectionGate';
import KeyValueRowsTable from './KeyValueRowsTable.vue';
import KeyValueToolbar from './KeyValueToolbar.vue';
import { rowMenu } from './menu';
import { useKeyValueViewStore } from './state';
import { useKeyValueActions } from './useKeyValueActions';
import { useKeyValueGate } from './useKeyValueGate';

const keyValueViewStore = useKeyValueViewStore();

const props = defineProps<{
  viewKey: string;
  tab?: KeyValueTabRecord;
}>();

const gate = useKeyValueGate(() => props.viewKey);
const { host, rt, page, connRecord, targetTail } = gate;
const actions = useKeyValueActions(() => props.viewKey, gate);
const toolbar = useTemplateRef<InstanceType<typeof KeyValueToolbar>>('toolbar');

// useConnectionGate needs a stable {id, connectionId}, which only a real tab record guarantees.
const connectionGate = props.tab
  ? useConnectionGate(
      () => props.tab as KeyValueTabRecord,
      () => keyValueViewStore.load(props.viewKey),
    )
  : null;
const needsReconnect = computed(() => connectionGate?.needsReconnect.value ?? false);
const reconnecting = computed(() => connectionGate?.reconnecting.value ?? false);
function onReconnectAndLoad(): Promise<boolean> {
  return connectionGate ? connectionGate.onReconnectAndLoad() : Promise.resolve(false);
}

usePublishEditDirty(
  () => props.viewKey,
  () => actions.objectDraft.value !== null || !!toolbar.value?.dirty,
);

const running = computed(() => rt.value?.status === 'loading');
const connColor = computed(() => connRecord.value?.color);
const iconClass = computed(() => connTextClass(connColor.value) ?? 'text-info');
const runState = useRunState(() => props.tab?.id);

// "connection / dbN / " (redis) or "connection / bucket / " (s3): the path's top-level segment.
const dbLabel = computed(() => {
  const connectionId = host.value?.connectionId;
  const path = host.value?.path;
  if (!connectionId || path === undefined) return null;
  try {
    return (
      decodePath(connectionId, path).segments.find(
        (s) => s.kind === 'database' || s.kind === 'bucket',
      )?.name ?? null
    );
  } catch {
    return null;
  }
});
const pathPrefix = computed(() =>
  dbLabel.value
    ? `${connRecord.value?.name ? `${connRecord.value.name} / ` : ''}${dbLabel.value} / `
    : '',
);

function rowMenuFor(field: string, value: string) {
  const p = page.value;
  if (!p) return null;
  const isObject = p.redisType === 'object';
  return rowMenu({
    field,
    value,
    redisType: p.redisType,
    canUpdate: gate.canUpdate.value,
    canDelete: gate.canDelete.value,
    canDownload: gate.canDownload.value,
    editable: isObject ? gate.objectEditGate.value.editable : p.redisType === 'string',
    editUnavailableLabel: isObject
      ? gate.objectEditGate.value.reason
      : t('studio.keyValue.editStringOnly.tooltip'),
    onEdit: () => (isObject ? actions.editObjectBody() : toolbar.value?.openEdit(value)),
    onDelete: () => void actions.onDeleteKey(),
    onDownload: () => void actions.onDownload(),
  });
}

function onRefresh(): void {
  refreshOrReconnect(needsReconnect.value, onReconnectAndLoad, () =>
    keyValueViewStore.reload(props.viewKey),
  );
}
function onToggleSearch(): void {
  keyValueViewStore.toggleSearchOpen(props.viewKey);
}

let unregisterCommand: (() => void) | null = null;
let unregisterFindCommand: (() => void) | null = null;

// The browse preview pane keeps one constant viewKey while the resolved target path changes, so
// loading follows the path (a tab fires once, immediately) instead of mount. A remount whose held
// page already shows the path keeps it, including the page index.
watch(
  () => (needsReconnect.value ? undefined : host.value?.path),
  (path) => {
    if (path === undefined || keyValueViewStore.hasPageFor(props.viewKey, path)) return;
    void keyValueViewStore.load(props.viewKey);
  },
  { immediate: true },
);

onMounted(() => {
  if (!props.tab) return;
  unregisterCommand = registerCommand('view.refresh', onRefresh);
  unregisterFindCommand = registerCommand('view.find', onToggleSearch);
});

onUnmounted(() => {
  unregisterCommand?.();
  unregisterFindCommand?.();
});
</script>

<template>
  <div class="flex-1 min-h-0 flex flex-col" data-testid="keyvalue-pane">
    <template v-if="tab">
      <ViewToolbar>
        <span v-if="connColor !== undefined" :class="colorMarkClass('dot', connColor)" />
        <span v-if="connRecord?.kind" class="size-4 flex items-center justify-center shrink-0">
          <EngineIcon :kind="connRecord.kind" :size="13" />
        </span>
        <span class="size-4 flex items-center justify-center shrink-0" :class="iconClass">
          <CodiconIcon :name="page?.redisType === 'object' ? 'file' : 'key'" :size="13" />
        </span>
        <span class="text-kira-md text-fg truncate" data-testid="keyvalue-target"
          ><span v-if="pathPrefix" class="text-subtle">{{ pathPrefix }}</span>
          {{ targetTail?.name ?? tab.path }}</span
        >
        <span class="ml-auto flex items-center gap-1" />
      </ViewToolbar>
      <div :class="colorMarkClass('band', connColor)" />
      <ViewToolbar>
        <div class="flex items-center gap-1.5 min-w-0">
          <TooltipIconButton
            :icon="ICON.refresh"
            :label="t('common.action.refresh.label')"
            data-testid="keyvalue-refresh"
            @click="onRefresh"
          />
          <TooltipIconButton
            :icon="ICON.stop"
            :label="t('common.action.stop.label')"
            disabled-trigger
            :class="{ 'is-live': !!running }"
            data-testid="keyvalue-stop"
            :disabled="!running"
            @click="keyValueViewStore.stop(viewKey)"
          />
        </div>
        <span class="ml-auto" />
        <RunState :state="runState" />
      </ViewToolbar>
    </template>

    <div class="flex flex-1 min-h-0 flex-col">
      <div class="flex min-h-0 flex-1 flex-col">
        <EmptyState
          v-if="needsReconnect && !gate.page.value"
          data-testid="keyvalue-reconnect"
          icon="plug"
          :title="t('studio.common.disconnected.empty')"
        >
          <Button
            variant="dialog-primary"
            size="kira-lg"
            data-testid="keyvalue-reconnect-load"
            :pending="reconnecting"
            @click="onReconnectAndLoad"
          >
            {{ t('studio.common.reconnectAndLoad.label') }}
          </Button>
        </EmptyState>
        <template v-else>
          <StaleDataStrip
            v-if="tab && gate.stale.value"
            testid="keyvalue-stale"
            :reconnecting="reconnecting"
            @reconnect="onReconnectAndLoad"
          />
          <KeyValueToolbar
            ref="toolbar"
            :view-key="viewKey"
            @delete="actions.onDeleteKey"
            @download="actions.onDownload"
            @upload="actions.onUpload"
            @edit-object="actions.editObjectBody"
          />

          <Alert
            v-if="rt?.status === 'error' && rt.error"
            variant="error"
            strip
            data-testid="keyvalue-error"
          >
            <AlertDescription>{{ rt.error.message }}</AlertDescription>
          </Alert>
          <Alert v-if="rt?.actionError" variant="error" strip data-testid="keyvalue-action-error">
            <AlertDescription>{{ rt.actionError }}</AlertDescription>
          </Alert>
          <Alert
            v-if="
              gate.isSingleObjectPage.value &&
              page &&
              gate.objectBodyRow.value === null &&
              page.memoryBytes !== null
            "
            variant="warn"
            strip
            data-testid="keyvalue-object-too-large"
          >
            <AlertDescription>
              {{
                t('studio.keyValue.tooLarge.description', {
                  size: formatBytes(page.memoryBytes),
                  limit: formatBytes(SINGLE_ROW_MAX_BYTES),
                })
              }}
            </AlertDescription>
          </Alert>
          <Alert
            v-if="actions.objectDraft.value !== null"
            variant="warn"
            strip
            data-testid="keyvalue-object-edit-pending"
          >
            <AlertDescription>
              <span data-testid="keyvalue-object-edit-note">{{
                actions.objectSaveError.value
                  ? t('studio.keyValue.objectEdit.pendingError.status', {
                      error: actions.objectSaveError.value,
                    })
                  : t('studio.keyValue.objectEdit.pending.status')
              }}</span>
            </AlertDescription>
            <AlertAction>
              <Button
                variant="toolbar-primary"
                size="kira"
                data-testid="keyvalue-object-edit-save"
                :pending="actions.objectSaving.value"
                :disabled="gate.stale.value"
                @click="actions.saveObjectEdit"
              >
                {{ t('common.action.save.label') }}
              </Button>
            </AlertAction>
          </Alert>

          <KeyValueRowsTable
            :view-key="viewKey"
            :menu-for="rowMenuFor"
            @select="actions.selectRow"
          />
        </template>
      </div>
      <CellEditorDock :tab-id="viewKey" />
    </div>
  </div>
</template>
