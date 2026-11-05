<script setup lang="ts">
import TooltipDetails from '@theme/components/TooltipDetails.vue';
import { t } from '@theme/i18n';
import { useAgentInfo } from '@workbench/agent/agentInfo';
import { useAutomationsModule } from '@workbench/automations/module';
import RunsStatusItem from '@workbench/automations/runs/RunsStatusItem.vue';
import AppMetricsItem from '@workbench/components/AppMetricsItem.vue';
import RunningOpsStatusItem from '@workbench/components/RunningOpsStatusItem.vue';
import StatusBarBase from '@workbench/components/StatusBar.vue';
import StatusBarItem from '@workbench/components/StatusBarItem.vue';
import UpdateAvailableItem from '@workbench/components/UpdateAvailableItem.vue';
import { type RunningOpItem, useRunningAutomationItems } from '@workbench/state/runningOps';
import { computed } from 'vue';
import AdeAgentsStatusItem from '../ade/v2/status/AdeAgentsStatusItem.vue';
import { useAppMetricsStore } from '../state/appMetrics';
import { useAppUpdateStore } from '../state/appUpdate';
import { useBlameStatusStore } from '../state/blameStatus';
import { useModeStore } from '../state/mode';
import { useOpsStore } from '../state/ops';
import { useSettingsStore } from '../state/settings';
import { blameLineText, blameLineTooltip } from '../views/repo/blameLine';
import AgentUsageItem from './AgentUsageItem.vue';

// Kira Studio's own workbench/StatusBar.vue, trimmed to the blame item on the left. Now a thin
// composition over the shared bar chrome (packages/workbench/src/components/StatusBar.vue). The
// app-metrics item is added back — AppMetricsItem.vue, shared with Kira Studio's own copy of this
// file. The update item is added, same shared component (UpdateAvailableItem.vue) and the same
// in-app dialog Kira Studio's own copy opens.
const blameStatusStore = useBlameStatusStore();
const appMetricsStore = useAppMetricsStore();
const appUpdateStore = useAppUpdateStore();
const modeStore = useModeStore();
const settingsStore = useSettingsStore();
const { info: agentInfo } = useAgentInfo();
const opsStore = useOpsStore();
const automations = useAutomationsModule();
const automationItems = useRunningAutomationItems();

// Background rows are sub-second tracker reads the panel hides by default; the widget skips them too.
const opItems = computed<RunningOpItem[]>(() =>
  opsStore.records
    .filter((r) => r.status === 'running' && !r.background)
    .map((r) => ({
      id: r.id,
      label: r.kind,
      detail: r.subject || r.repoName,
      startedAt: Date.parse(r.startedAt),
      canStop: r.cancellable,
      stop: () => void opsStore.cancelOp(r.id),
    })),
);
const items = computed(() => [...opItems.value, ...automationItems.value]);

// P76 §5.2: 'none' and 'uncommitted' both render nothing — an always-present "Uncommitted" readout
// is the extension's own choice; this bar hides items with nothing to say instead.
const blame = computed(() =>
  blameStatusStore.status.kind === 'resolved' ? blameStatusStore.status : null,
);
const blameText = computed(() => (blame.value ? blameLineText(blame.value) : ''));
const blameTooltip = computed(() => (blame.value ? blameLineTooltip(blame.value) : null));

function onRevealBlameCommit(): void {
  if (blame.value) blameStatusStore.reveal?.(blame.value.sha);
}
</script>

<template>
  <StatusBarBase>
    <template #left-extra>
      <!-- P76 §5.2: left-side item. -->
      <StatusBarItem
        v-if="blame"
        icon="git-commit"
        :disabled="!blameStatusStore.reveal"
        data-testid="blame-status"
        @click="onRevealBlameCommit"
      >
        <span class="max-w-80 truncate">{{ blameText }}</span>
        <template #tip>
          <TooltipDetails
            v-if="blameTooltip"
            :title="blameTooltip.subject"
            :rows="[
              { label: t('git.search.field.author.label'), value: blameTooltip.author },
              { label: t('common.field.date.label'), value: blameTooltip.date },
            ]"
            :hint="blameStatusStore.reveal ? t('space.repo.blame.reveal.hint') : undefined"
          />
        </template>
      </StatusBarItem>
      <!-- P239: agent usage limits, ADE module only, for a provider that reports them. -->
      <AgentUsageItem
        v-if="modeStore.active === 'ade' && settingsStore.agent.usageEnabled && agentInfo.usage"
      />
    </template>
    <template #right>
      <AdeAgentsStatusItem />
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
