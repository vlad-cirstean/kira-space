<script setup lang="ts">
import { useAutomationsModule } from '@workbench/automations/module';
import RunsStatusItem from '@workbench/automations/runs/RunsStatusItem.vue';
import AppMetricsItem from '@workbench/components/AppMetricsItem.vue';
import RunningOpsStatusItem from '@workbench/components/RunningOpsStatusItem.vue';
import StatusBarBase from '@workbench/components/StatusBar.vue';
import UpdateAvailableItem from '@workbench/components/UpdateAvailableItem.vue';
import { type RunningOpItem, useRunningAutomationItems } from '@workbench/state/runningOps';
import { computed } from 'vue';
import { useAppMetricsStore } from '../state/appMetrics';
import { useAppUpdateStore } from '../state/appUpdate';
import { useConnectionsStore } from '../state/connections';
import { useOpsStore } from '../state/ops';
import { TAB_KINDS } from '../state/tabKinds';
import { useTabsStore } from '../state/tabs';

// Kira Studio's own StatusBar.vue, now a thin composition over the shared bar
// chrome (packages/workbench/src/components/StatusBar.vue) — this file keeps exactly the per-app
// right-side items: update/app-metrics. The app-metrics item's
// own markup moved to AppMetricsItem.vue, shared with Kira Space's own copy. The update item's
// own markup moved to UpdateAvailableItem.vue the same way — its click now opens the in-app dialog
// (appUpdateStore.openUpdateDialog) instead of the release page. The session-status
// widget this file also carried moved out with the rest of agent-activity monitoring — no app shows
// it.
const appMetricsStore = useAppMetricsStore();
const appUpdateStore = useAppUpdateStore();
const opsStore = useOpsStore();
const connectionsStore = useConnectionsStore();
const tabsStore = useTabsStore();
const automations = useAutomationsModule();
const automationItems = useRunningAutomationItems();

// Built from the raw records, not visibleOps: the panel's filters must not hide running work.
const opItems = computed<RunningOpItem[]>(() =>
  opsStore.records
    .filter((r) => r.status === 'running')
    .map((r) => {
      const tab = r.tabId ? tabsStore.tabs.find((x) => x.id === r.tabId) : undefined;
      return {
        id: r.id,
        label: connectionsStore.connectionRecord(r.connectionId)?.name ?? r.kind,
        detail: tab ? `${r.kind} · ${TAB_KINDS[tab.kind].title(tab)}` : r.kind,
        startedAt: Date.parse(r.startedAt),
        canStop: true,
        stop: () => void opsStore.cancelOp(r.id),
      };
    }),
);
const items = computed(() => [...opItems.value, ...automationItems.value]);
</script>

<template>
  <StatusBarBase>
    <template #right>
      <RunningOpsStatusItem :items="items" @open="automations.showOperations()" />
      <RunsStatusItem />
      <UpdateAvailableItem
        v-if="appUpdateStore.available"
        :latest-version="appUpdateStore.latestVersion"
        :current-version="appUpdateStore.currentVersion"
        @open="appUpdateStore.openUpdateDialog()"
      />
      <AppMetricsItem :sample="appMetricsStore.sample" :app-name="appUpdateStore.appName" />
    </template>
  </StatusBarBase>
</template>
