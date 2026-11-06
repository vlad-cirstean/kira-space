<script setup lang="ts">
import { pathTail } from '@shared/domain/tree';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverAnchor } from '@theme/components/ui/popover';
import { colorMarkClass, connTextClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import RunState from '@theme/RunState.vue';
import { errorText } from '@workbench/bridge/codedError';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { registerCommand } from '@workbench/shortcuts/commands';
import { computed, onMounted, onUnmounted, ref } from 'vue';
import type { DataTabRecord } from '../../../state/tabDomain';
import { useConnectionsStore } from '../../state/connections';
import { useFakeDataStore } from '../../state/fakeData';
import { nodeKindLabel } from '../../state/nodeKindLabel';
import { useRunState } from '../../state/runState';
import EngineIcon from '../../theme/EngineIcon.vue';
import CellEditorDock from '../shared/celleditor/CellEditorDock.vue';
import SearchToolbar from '../shared/page/SearchToolbar.vue';
import StaleDataStrip from '../shared/StaleDataStrip.vue';
import { ancestorPathPrefix } from '../shared/targetPath';
import { refreshOrReconnect, useConnectionGate } from '../shared/useConnectionGate';
import DataToolbar from './DataToolbar.vue';
import FilterToolbar from './FilterToolbar.vue';
import { canGenerateDataFor } from './fakeData/generate';
import { flushOpenEdit } from './openEdit';
import PreviewCommandPanel from './PreviewCommandPanel.vue';
import { getPage, pageVersion } from './page';
import { usePendingChangesStore } from './pendingChanges';
import SlickGridHost from './SlickGridHost.vue';
import { type Match, pageSearchApi } from './search';
import { useGridViewStore } from './state';

// MainView.vue keys this component by tab.id, so one instance <-> one tab: onMounted below
// fires fresh on every tab switch, which is what makes per-tab load-on-activate and scroll
// restore (SlickGridHost's own onMounted) work without a manual watcher.
const fakeDataStore = useFakeDataStore();
const pendingChangesStore = usePendingChangesStore();
const connectionsStore = useConnectionsStore();
const gridViewStore = useGridViewStore();
const props = defineProps<{ tab: DataTabRecord }>();

const { needsReconnect, reconnecting, onReconnectAndLoad } = useConnectionGate(
  () => props.tab,
  () => gridViewStore.load(props.tab.id),
);

const rt = computed(() => gridViewStore.runtime[props.tab.id]);

// Shown page outlived its connection: kept, read-only, behind StaleDataStrip.
const stale = computed(() => gridViewStore.isStale(props.tab.id));
const hasPage = computed(() => {
  void pageVersion.n;
  return !!getPage(props.tab.id);
});

const connRecord = computed(() => connectionsStore.connectionRecord(props.tab.connectionId));

const iconClass = computed(() => connTextClass(connRecord.value?.color) ?? 'text-muted-foreground');

// P104 §3: ViewChrome/ViewHeader/RunState inlined at this call site (no library counterpart).
const railColor = computed(() => (connRecord.value ? (connRecord.value.color ?? null) : undefined));
const runState = useRunState(() => props.tab.id);

const targetTail = computed(() => pathTail(props.tab.path));

const KIND_ICON: Record<string, string> = {
  table: 'table',
  view: 'eye',
  matview: 'symbol-structure',
};
const targetIcon = computed(() => {
  const kind = targetTail.value?.kind;
  return (kind && KIND_ICON[kind]) || 'table';
});

// P48 F20: shared with DocumentView.vue's own identical breadcrumb prefix.
const pathPrefix = computed(() => ancestorPathPrefix(props.tab.connectionId, props.tab.path));

// The grid was the one view whose header carried no facts at all — every sibling view
// (KeyValue has four badges, Definition two, Documents one) says more about what's on screen than
// this one did. Kind + column count + read-only state are always known the moment a tab opens;
// the row count is gated on rt.count existing at all (computing one automatically is forbidden), so
// this badge appears only once the user has pressed Σ, never before.
const columnCount = computed(() => rt.value?.meta?.columns.length ?? null);
const isWritable = computed(() => !!caps.value?.writable && !connRecord.value?.readOnly);
// M5 §6.4: the grid-writable-badge's own reason — masked reads as read-only, since it is (the
// preview toggle can't be on while anything is pending, so this never has to distinguish "masked
// but would otherwise be writable" from "genuinely read-only" for the badge's one-word purpose).
const effectivelyWritable = computed(
  () => isWritable.value && !rt.value?.maskPreview && !stale.value,
);
const primaryKeyLabel = computed(() => {
  const names = rt.value?.meta?.columns.filter((c) => c.isPrimaryKey).map((c) => c.name) ?? [];
  return names.length ? t('studio.grid.primaryKey.status', { names: names.join(', ') }) : null;
});

// The pending-changes group is here so it can land in ViewChrome's own #toolbar-end slot
// (after the chrome's automatic push and RunState too) — a component mounted inside #toolbar
// cannot also render into a sibling named slot of its parent.
const caps = computed(() => {
  const connectionId = props.tab.connectionId;
  return connectionId ? (connectionsStore.states[connectionId]?.caps ?? null) : null;
});
const tabHasPending = computed(() => pendingChangesStore.hasPending(props.tab.id));
const pendingCount = computed(() => {
  const p = pendingChangesStore.pendingFor(props.tab.id);
  if (!p) return 0;
  return p.edits.size + p.deletes.size + p.inserts.length;
});
const offPageCount = computed(() => pendingChangesStore.offPageCount(props.tab.id));
const committing = computed(() => pendingChangesStore.isCommitting(props.tab.id));
const previewOpen = ref(false);
const previewAnchorRef = ref<HTMLElement | null>(null);

// commitPending's own rejection (a constraint violation, a type error, a read-only
// refusal) used to be an unhandled promise rejection — no try/catch here and no async-aware
// @click. The staged set already survives a failure (clearPending only runs on success); what was
// missing was telling the user why.
async function onCommit(): Promise<void> {
  if (!props.tab.connectionId || stale.value) return;
  // An open inline edit stages here, before the plan is built and the store starts refusing edits.
  flushOpenEdit(props.tab.id);
  try {
    await pendingChangesStore.commitPending(
      props.tab.connectionId,
      props.tab.path,
      props.tab.id,
      async () => {
        gridViewStore.setActionError(props.tab.id, null);
        await gridViewStore.reloadAfterMutation(props.tab.id);
      },
    );
  } catch (err) {
    gridViewStore.setActionError(props.tab.id, errorText(err));
  }
}

function onDiscard(): void {
  pendingChangesStore.discardPending(props.tab.id);
  // P43 F5/D7: a discard resolves the very staging that a prior actionError was about — an error
  // strip surviving it would be pointing at a change that no longer exists.
  gridViewStore.setActionError(props.tab.id, null);
}

function onRefresh(): void {
  refreshOrReconnect(needsReconnect.value, onReconnectAndLoad, () =>
    gridViewStore.reload(props.tab.id),
  );
}
function onStop(): void {
  gridViewStore.stop(props.tab.id);
}

// P15 D1/D11: the palette's own gate, since the palette entry has no disabled-button affordance
// to lean on — shares canGenerateDataFor with DataToolbar.vue's own button (P12 round 1 F16) so
// the two can't drift apart the way this file's own predicate once did.
function onGenerateData(): void {
  // M5 §6.4: the same lockout DataToolbar.vue's own Generate button applies — this is the
  // command-palette/keyboard-shortcut path to the identical action, and must not bypass it.
  if (
    !canGenerateDataFor(caps.value, connRecord.value?.readOnly) ||
    rt.value?.maskPreview ||
    stale.value
  )
    return;
  fakeDataStore.openGenerateDataDialog(props.tab.id);
}

let unregisterCommands: Array<() => void> = [];

onMounted(() => {
  if (!needsReconnect.value && !gridViewStore.runtime[props.tab.id]) {
    void gridViewStore.load(props.tab.id);
  }
  // D11: this component is mounted only while its tab is the active one (MainView.vue's
  // `v-else-if` chain), so registering here — rather than switching on tab kind in a global
  // dispatcher — is what makes Find/Refresh always act on the currently visible data tab.
  unregisterCommands = [
    registerCommand('view.find', () => gridViewStore.toggleSearchOpen(props.tab.id)),
    // This used to call reload() directly, a doomed
    // no-op while the tab sits behind the reconnect gate (same bug the toolbar's own Refresh
    // button had) — the keyboard-shortcut/command-palette path needs the
    // same reconnect-or-refresh semantics as the visible button, not a second, unguarded one.
    registerCommand('view.refresh', () => onRefresh()),
    registerCommand('data.generate', onGenerateData),
  ];
});

onUnmounted(() => {
  for (const off of unregisterCommands) off();
});

const dataGridRef = ref<{ scrollCellIntoView: (row: number, col: number) => void } | null>(null);

function onGoToMatch(match: Match): void {
  dataGridRef.value?.scrollCellIntoView(match.row, match.col);
}
function onCloseSearch(): void {
  gridViewStore.setSearchOpen(props.tab.id, false);
}
</script>

<template>
  <div class="h-full flex flex-col min-h-0">
    <ViewToolbar data-testid="view-head">
      <span
        v-if="railColor !== undefined"
        :class="colorMarkClass('dot', railColor)"
        data-testid="conn-dot"
      />
      <span v-if="connRecord?.kind" class="size-4 flex items-center justify-center shrink-0"
        ><EngineIcon :kind="connRecord.kind" :size="13" /></span
      >
      <span class="size-4 flex items-center justify-center shrink-0" :class="iconClass"
        ><CodiconIcon :name="targetIcon" :size="13" /></span
      >
      <span class="text-kira-md text-fg truncate" data-testid="grid-target">
        <span v-if="pathPrefix" class="text-subtle">{{ pathPrefix }}</span>
        {{ targetTail?.name ?? tab.path }}
      </span>
      <Badge v-if="targetTail?.kind" data-testid="grid-kind-badge">{{
        nodeKindLabel(targetTail.kind)
      }}</Badge>
      <Badge v-if="columnCount !== null" data-testid="grid-column-count-badge">{{
        t('studio.grid.columnCount.status', { count: columnCount })
      }}</Badge>
      <Badge data-testid="grid-writable-badge">{{
        effectivelyWritable ? t('studio.grid.writable.status') : t('common.state.readOnly.status')
      }}</Badge>
      <Badge v-if="rt?.count" data-testid="grid-row-count-badge">{{
        t('studio.grid.rowCount.status', { count: rt.count.value })
      }}</Badge>
      <span class="ml-auto flex items-center gap-1">
        <Badge data v-if="primaryKeyLabel" variant="info" data-testid="grid-pk-chip">{{
          primaryKeyLabel
        }}</Badge>
      </span>
    </ViewToolbar>

    <div :class="colorMarkClass('band', railColor)" data-testid="toolbar-rail" />
    <ViewToolbar data-testid="data-toolbar">
      <div class="flex items-center gap-1.5 min-w-0">
        <TooltipIconButton
          :icon="ICON.refresh"
          :label="t('common.action.refresh.label')"
          disabled-trigger
          data-testid="toolbar-refresh"
          :disabled="!!rt?.opId"
          @click="onRefresh"
        />
        <TooltipIconButton
          :icon="ICON.stop"
          :label="t('common.action.stop.label')"
          disabled-trigger
          :class="{ 'text-error': !!rt?.opId }"
          data-testid="toolbar-stop"
          :disabled="!rt?.opId"
          @click="onStop"
        />
      </div>
      <DataToolbar :tab="tab" />
      <span class="ml-auto" />
      <RunState :state="runState" />
      <!-- FIX-3: pending edits as a count with both actions beside it — Commit is the only
           accent-filled control on the whole screen. The preview-command eye sits in this same
           group. -->
      <div class="flex items-center gap-1.5 min-w-0">
        <template v-if="tabHasPending">
          <Badge variant="warn" data-testid="pending-count-chip">{{
            t('studio.grid.pending.rows.status', { count: pendingCount })
          }}</Badge>
          <span
            v-if="offPageCount > 0"
            class="text-kira-sm text-muted-foreground truncate"
            data-testid="pending-off-page"
            >{{
              t('studio.grid.pending.offPage.status', { count: offPageCount })
            }}</span
          >
          <Popover :open="previewOpen" @update:open="previewOpen = $event">
            <div ref="previewAnchorRef" class="relative">
              <TooltipIconButton
                icon="eye"
                :label="
                  stale
                    ? t('studio.common.connectionLost.tooltip')
                    : isWritable
                      ? t('studio.grid.previewCommand.label')
                      : t('studio.common.connectionReadOnly.tooltip')
                "
                :aria-label="t('studio.grid.previewCommand.label')"
                disabled-trigger
                data-testid="toolbar-preview-command"
                :disabled="!isWritable || committing || stale"
                @click="previewOpen = !previewOpen"
              />
              <PopoverAnchor :reference="previewAnchorRef ?? undefined" />
            </div>
            <PreviewCommandPanel v-if="previewOpen" :tab-id="tab.id" @close="previewOpen = false" />
          </Popover>
          <TooltipIconButton
            :icon="ICON.reset"
            :label="t('studio.grid.discardPending.label')"
            disabled-trigger
            data-testid="toolbar-discard-changes"
            :disabled="committing || (!isWritable && !stale)"
            @click="onDiscard"
          />
          <TooltipIconButton
            icon="save"
            :label="
              stale
                ? t('studio.common.connectionLost.tooltip')
                : t('studio.grid.commitPending.label')
            "
            :aria-label="t('studio.grid.commitPending.label')"
            disabled-trigger
            variant="dialog-primary"
            data-testid="toolbar-commit-changes"
            :disabled="!isWritable || committing || stale"
            @click="onCommit"
          />
        </template>
      </div>
    </ViewToolbar>
    <ViewToolbar data-testid="filter-toolbar">
      <FilterToolbar :tab="tab" />
    </ViewToolbar>

    <!-- Below the filter row, not floating over the grid it searches — the "docks at the bottom
         of the result" placement from Toolbars.html overlapped the last visible row, which read as
         a bug rather than a search bar. -->
    <SearchToolbar
      v-if="rt?.searchOpen"
      :tab-id="tab.id"
      :is-open="() => !!gridViewStore.runtime[tab.id]?.searchOpen"
      testid-prefix=""
      kind="rows"
      :api="pageSearchApi"
      :mask-preview-on="!!rt?.maskPreview"
      @go-to-match="onGoToMatch"
      @close="onCloseSearch"
    />

    <div class="flex flex-1 min-h-0 flex-col">
      <div class="flex min-h-0 flex-1 flex-col">
        <EmptyState
          v-if="needsReconnect && !hasPage"
          data-testid="reconnect-panel"
          icon="plug"
          :title="t('studio.common.disconnected.empty')"
        >
          <Button
            variant="dialog-primary"
            size="kira-lg"
            data-testid="reconnect-load"
            :pending="reconnecting"
            @click="onReconnectAndLoad"
          >
            {{ t('studio.common.reconnectAndLoad.label') }}
          </Button>
        </EmptyState>
        <template v-else>
          <StaleDataStrip
            v-if="stale"
            testid="grid-stale"
            :reconnecting="reconnecting"
            @reconnect="onReconnectAndLoad"
          />
          <!-- Work-in-progress is the ring + elapsed time beside the button
               that started it (RunState, above), never a bar across the top of the view —
               "never a spinner that replaces the previous page" still holds, it no longer needs a
               bar of its own to say so. -->
          <Alert
            strip
            v-if="rt?.status === 'error' && rt.error"
            variant="error"
            data-testid="error-strip"
            class="whitespace-pre-wrap font-data"
          >
            <AlertDescription>{{ rt.error.message }}</AlertDescription>
          </Alert>
          <!-- P43 F5/D7: a failed commit, distinct from a failed load above — the grid is still
               showing a perfectly valid page, only the write was refused. -->
          <Alert
            strip
            v-if="rt?.actionError"
            variant="error"
            data-testid="data-action-error"
            class="whitespace-pre-wrap font-data"
          >
            <AlertDescription>{{ rt.actionError }}</AlertDescription>
          </Alert>
          <!-- The one place this view states, in prose, that what's on screen is not the
               stored data — a preview convenience, not the security boundary, but a user
               switching tabs or taking a screenshot must not mistake a bucket string for a literal
               value. -->
          <Alert
            strip
            v-if="rt?.maskPreview"
            variant="info"
            icon="eye-closed"
            data-testid="mask-preview-strip"
          >
            <AlertDescription>{{ t('studio.grid.maskPreview.description') }}</AlertDescription>
          </Alert>
          <div class="flex-1 min-h-0 relative">
            <SlickGridHost ref="dataGridRef" :tab-id="tab.id" />
          </div>
        </template>
      </div>
      <CellEditorDock :tab-id="tab.id" />
    </div>
  </div>
</template>
