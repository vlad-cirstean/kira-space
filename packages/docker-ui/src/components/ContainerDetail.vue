<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import Spinner from '@theme/components/Spinner.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Skeleton } from '@theme/components/ui/skeleton';
import { type MessageKey, t } from '@theme/i18n';
import { formatBytes, formatPercent } from '@workbench/behaviour/format';
import Identifier from '@workbench/components/Identifier.vue';
import QueryState from '@workbench/components/QueryState.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, ref, watch } from 'vue';
import { publishedPorts, shortId, stateDotTone } from '../lib/format';
import type { ContainerAction } from '../queries';
import { useContainerAction, useContainerDetail, useRegistryLink } from '../queries';
import { useDockerEditStore } from '../state/dockerEdit';
import { useDockerStatsStore } from '../state/dockerStats';
import { type DockerDetailTab, useDockerUiStore } from '../state/dockerUi';
import ContainerEditView from './ContainerEditView.vue';
import ContainerOverview from './ContainerOverview.vue';
import ExecView from './ExecView.vue';
import InspectView from './InspectView.vue';
import LogsView from './LogsView.vue';
import StatsView from './StatsView.vue';
import UsageBar from './UsageBar.vue';

const props = defineProps<{ containerId: string }>();

const ui = useDockerUiStore();
const stats = useDockerStatsStore();
const edits = useDockerEditStore();
const id = computed(() => props.containerId);
const detail = useContainerDetail(id);
const actions = useContainerAction();

const c = computed(() => detail.data.value?.container);
const registry = useRegistryLink(() => c.value?.imageId ?? '');
const running = computed(() => c.value?.state === 'running');
const isUp = running;
const live = computed(() => (running.value ? stats.latest.get(props.containerId) : undefined));
const recreating = computed(() => edits.applying[props.containerId] === 'recreate');
const inFlight = computed(
  () => actions.isBusy(props.containerId) || edits.applying[props.containerId] !== undefined,
);
const lastAction = ref<ContainerAction | undefined>(undefined);
const pendingOf = (action: ContainerAction): boolean =>
  inFlight.value && lastAction.value === action;
const lockedBy = (action: ContainerAction): boolean =>
  inFlight.value && lastAction.value !== action;

const TABS: ReadonlyArray<{
  id: DockerDetailTab;
  label: MessageKey;
  icon: string;
  needsRunning: boolean;
}> = [
  { id: 'overview', label: 'docker.tab.overview.label', icon: 'info', needsRunning: false },
  { id: 'logs', label: 'docker.tab.logs.label', icon: 'output', needsRunning: false },
  { id: 'terminal', label: 'common.module.terminal.label', icon: 'terminal', needsRunning: true },
  { id: 'stats', label: 'docker.tab.stats.label', icon: 'graph', needsRunning: false },
  { id: 'inspect', label: 'docker.tab.inspect.label', icon: 'json', needsRunning: false },
  { id: 'edit', label: 'common.action.edit.label', icon: 'edit', needsRunning: false },
];

const tabItems = computed(() =>
  TABS.map((tab) => ({
    value: tab.id,
    label: t(tab.label),
    icon: tab.icon,
    disabled: tab.needsRunning && !running.value,
    testid: `docker-tab-${tab.id}`,
  })),
);

const stateVariant = computed(() => {
  switch (c.value?.state) {
    case 'running':
      return 'ok';
    case 'paused':
    case 'restarting':
      return 'warn';
    case 'dead':
      return 'error';
    default:
      return 'default';
  }
});

// A stopped container cannot serve its Terminal tab: fall back to Overview.
watch([running, c], () => {
  if (c.value && !running.value && ui.detailTab === 'terminal') {
    ui.detailTab = 'overview';
  }
});

function act(action: ContainerAction): void {
  lastAction.value = action;
  void actions.run(props.containerId, action).catch(() => {
    // Surfaced through `actions.error`.
  });
}
</script>

<template>
  <div class="flex h-full flex-col" data-testid="docker-container-detail">
    <div
      v-if="recreating"
      class="flex items-center gap-2 p-4 text-muted-foreground"
      data-testid="docker-detail-recreating"
    >
      <Spinner label="" />{{ t('docker.detail.recreating.loading') }}
    </div>
    <QueryState v-else :query="detail" :is-empty="() => false">
      <template #error>
        <EmptyState
          fill
          data-testid="docker-detail-error"
          :title="t('docker.detail.notFound.title')"
          :description="t('docker.detail.notFound.description')"
        >
          <template #media><CodiconIcon name="warning" :size="24" class="text-warn" /></template>
          <Button size="kira" variant="toolbar" @click="ui.select(null)">{{
            t('docker.detail.backToOverview.label')
          }}</Button>
        </EmptyState>
      </template>
      <template #loading>
        <div class="flex flex-col gap-2 p-3" aria-busy="true">
          <Skeleton class="h-control-lg w-1/3" />
          <Skeleton class="h-4 w-2/3" />
        </div>
      </template>
      <template v-if="c" #default="{ data }">
        <div class="flex shrink-0 flex-col gap-1 px-3 py-2">
          <div class="flex min-w-0 items-center gap-2">
            <StatusDot :tone="stateDotTone(c.state)" />
            <span
              class="min-w-0 truncate text-kira-lg font-medium"
              data-testid="docker-detail-name"
              >{{
                c.name
              }}</span
            >
            <Badge :variant="stateVariant" data-testid="docker-detail-state">{{ c.state }}</Badge>
            <span class="ml-auto flex shrink-0 items-center gap-1">
              <Button
                size="kira"
                variant="toolbar"
                :disabled="isUp || lockedBy('start')"
                :pending="pendingOf('start')"
                data-testid="docker-action-start"
                @click="act('start')"
              >
                <CodiconIcon name="play" :size="12" class="text-ok" />
                {{ t('common.action.start.label') }}
              </Button>
              <Button
                size="kira"
                variant="toolbar"
                :disabled="!isUp || lockedBy('stop')"
                :pending="pendingOf('stop')"
                data-testid="docker-action-stop"
                @click="act('stop')"
              >
                <CodiconIcon name="debug-stop" :size="12" class="text-error" />
                {{ t('common.action.stop.label') }}
              </Button>
              <Button
                size="kira"
                variant="toolbar"
                :disabled="!isUp || lockedBy('restart')"
                :pending="pendingOf('restart')"
                data-testid="docker-action-restart"
                @click="act('restart')"
              >
                <CodiconIcon name="debug-restart" :size="12" />
                {{ t('common.action.restart.label') }}
              </Button>
            </span>
          </div>
          <div
            class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-kira-sm text-muted-foreground"
          >
            <span class="min-w-0 truncate" data-testid="docker-detail-image">{{ c.image }}</span>
            <TooltipIconButton
              v-if="registry.url.value"
              icon="link-external"
              :label="t('docker.detail.openRegistry.tooltip')"
              :disabled="registry.checking.value"
              data-testid="docker-detail-registry"
              @click="registry.open()"
            />
            <span v-if="registry.missing.value" data-testid="docker-registry-missing">{{
              t('docker.detail.registryMissing.status')
            }}</span>
            <Identifier :value="c.id" :short="shortId" copyable data-testid="docker-detail-id" />
            <span v-if="c.status">{{ c.status }}</span>
            <span
              v-for="p in publishedPorts(c.ports)"
              :key="p"
              class="font-data"
              data-testid="docker-detail-port"
              >:{{ p }}</span
            >
            <span
              v-if="live"
              class="ml-auto flex items-center gap-3"
              data-testid="docker-detail-live"
            >
              <span class="flex w-28 items-center gap-1.5">
                {{ t('docker.metric.cpu.label') }}
                <span class="font-data text-fg">{{ formatPercent(live.cpuPercent) }}</span>
                <UsageBar :percent="live.cpuPercent" class="flex-1" />
              </span>
              <span class="flex w-32 items-center gap-1.5">
                {{ t('docker.metric.ram.label') }}
                <span class="font-data text-fg">{{ formatBytes(live.memUsage) }}</span>
                <UsageBar :percent="live.memPercent" class="flex-1" />
              </span>
            </span>
          </div>
        </div>
        <Alert v-if="actions.error.value" variant="error" data-testid="docker-action-error">
          <AlertDescription>{{ (actions.error.value as Error).message }}</AlertDescription>
        </Alert>
        <ViewToolbar class="overflow-x-auto">
          <SecondaryTabs
            :model-value="ui.detailTab"
            :items="tabItems"
            :aria-label="t('docker.detail.sections.aria')"
            @update:model-value="(v) => (ui.detailTab = v as DockerDetailTab)"
          />
        </ViewToolbar>
        <div class="min-h-0 flex-1">
          <ContainerOverview v-if="ui.detailTab === 'overview'" :detail="data" />
          <LogsView v-else-if="ui.detailTab === 'logs'" :container-id="containerId" />
          <ExecView
            v-else-if="ui.detailTab === 'terminal' && running"
            :container-id="containerId"
          />
          <StatsView
            v-else-if="ui.detailTab === 'stats'"
            :container-id="containerId"
            :running="running"
          />
          <InspectView v-else-if="ui.detailTab === 'inspect'" :raw="data.raw" />
          <ContainerEditView v-else-if="ui.detailTab === 'edit'" :container-id="containerId" />
        </div>
      </template>
    </QueryState>
  </div>
</template>
