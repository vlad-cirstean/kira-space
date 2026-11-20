<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { Alert } from '@theme/components/ui/alert';
import { FieldDescription } from '@theme/components/ui/field';
import { formatRelative } from '@theme/format';
import { type MessageKey, t } from '@theme/i18n';
import QueryState from '@workbench/components/QueryState.vue';
import RelativeTime from '@workbench/components/RelativeTime.vue';
import { storeToRefs } from 'pinia';
import { computed } from 'vue';
import { type ConnectionStatus, connectionStatus, lastFrameAt } from '../api/events';
import ActivityAgentRow from '../components/ActivityAgentRow.vue';
import PhoneRunRow from '../components/PhoneRunRow.vue';
import { useAuthStore } from '../state/auth';
import { useAgentsModel } from '../state/useAgentsModel';
import { useComputer, usePhoneRuns } from '../state/usePhoneStatus';

// What is happening now: running agents and how long ago each moved, script runs, and the
// computer behind the app (connection, repos, Docker).
const { model, boardQuery, nowMs, repoLabel } = useAgentsModel();
const { permissions } = storeToRefs(useAuthStore());
const runsQuery = usePhoneRuns();
const computer = useComputer();

const FINISHED_RUNS = 10;
const runs = computed(() => {
  const all = runsQuery.data.value ?? [];
  return [
    ...all.filter((r) => r.state === 'running'),
    ...all.filter((r) => r.state !== 'running').slice(0, FINISHED_RUNS),
  ];
});

const summary = computed(() => {
  const s = model.value?.agents.summary;
  if (!s) return '';
  const parts = [t('space.statusBar.agents.label', { count: s.total })];
  if (s.byKind.working > 0)
    parts.push(t('space.ade.allSessions.summaryWorking.label', { count: s.byKind.working }));
  if (s.byKind.waiting > 0)
    parts.push(t('space.ade.allSessions.summaryWaiting.label', { count: s.byKind.waiting }));
  if (s.byKind.input > 0)
    parts.push(t('space.ade.allSessions.summaryInput.label', { count: s.byKind.input }));
  if (s.byKind.unseen > 0)
    parts.push(t('space.ade.allSessions.summaryUnseen.label', { count: s.byKind.unseen }));
  return parts.join(' · ');
});

const CONNECTION: Record<ConnectionStatus, { key: MessageKey; tone: 'ok' | 'warn' | 'error' }> = {
  live: { key: 'mobile.terminal.status.live.status', tone: 'ok' },
  reconnecting: { key: 'mobile.terminal.status.reconnecting.status', tone: 'warn' },
  offline: { key: 'mobile.activity.computer.connection.offline.status', tone: 'error' },
};
const DOCKER_REASON: Record<string, MessageKey> = {
  'not-installed': 'docker.unavailable.notInstalled.title',
  'daemon-down': 'docker.unavailable.daemonDown.title',
  'permission-denied': 'docker.unavailable.permissionDenied.title',
  unreachable: 'docker.unavailable.unreachable.title',
  tls: 'docker.unavailable.tls.title',
  error: 'docker.unavailable.error.title',
};
const docker = computed(() => {
  const d = computer.data.value?.docker;
  if (!d) return null;
  if (d.state === 'ok')
    return {
      ok: true,
      text: t('mobile.activity.computer.docker.counts.label', {
        running: d.running,
        stopped: d.stopped,
      }),
    };
  return {
    ok: false,
    text: t(DOCKER_REASON[d.reason] ?? 'docker.unavailable.error.title'),
  };
});
const fetchedText = (at: number | null): string =>
  at === null
    ? t('space.settings.configSync.neverFetched.status')
    : t('mobile.activity.repos.fetched.label', { ago: formatRelative(at, nowMs.value) });
</script>

<template>
  <section class="flex flex-col" data-testid="activity-screen">
    <SectionHeading :label="t('common.module.agents.label')" />
    <QueryState :query="boardQuery" :is-empty="() => false">
      <template #error>
        <Alert variant="error" strip data-testid="activity-error">{{
          t('mobile.needs.loadError.error')
        }}</Alert>
      </template>
      <template v-if="model">
        <EmptyState
          v-if="!model.agents.views.length"
          icon="robot"
          data-testid="activity-agents-empty"
          :title="t('mobile.activity.agents.empty')"
        />
        <template v-else>
          <FieldDescription class="px-3 py-2" data-testid="activity-summary">{{
            summary
          }}</FieldDescription>
          <ul class="m-0 flex list-none flex-col p-0">
            <ActivityAgentRow
              v-for="view in model.agents.views"
              :key="view.session.id"
              :view="view"
              :title="model.cards.get(view.session.taskId)?.title ?? view.session.taskId"
              :now-ms="nowMs"
              :can-attach="permissions.agentInput"
            />
          </ul>
        </template>
      </template>
    </QueryState>

    <SectionHeading :label="t('common.scriptRuns.label')" />
    <QueryState :query="runsQuery" :is-empty="() => false" variant="inline">
      <template #error>
        <Alert variant="error" strip data-testid="activity-runs-error">{{
          t('mobile.activity.runs.loadError.error')
        }}</Alert>
      </template>
      <InlineState v-if="!runs.length" kind="empty" data-testid="activity-runs-empty">{{
        t('mobile.activity.runs.empty')
      }}</InlineState>
      <ul v-else class="m-0 flex list-none flex-col p-0" data-testid="activity-runs">
        <PhoneRunRow v-for="run in runs" :key="run.id" :run="run" />
      </ul>
    </QueryState>

    <SectionHeading :label="t('mobile.activity.computer.title')" />
    <ul class="m-0 flex list-none flex-col p-0" data-testid="activity-computer">
      <li class="flex items-center gap-2 border-b border-border px-3 py-3 text-kira-md">
        <span class="flex-1">{{ t('common.field.connection.label') }}</span>
        <StatusDot :tone="CONNECTION[connectionStatus].tone" />
        <span data-testid="activity-connection">{{ t(CONNECTION[connectionStatus].key) }}</span>
        <span
          v-if="connectionStatus !== 'live' && lastFrameAt !== null"
          class="text-kira-sm text-subtle"
          >{{ t('mobile.shell.updated.label') }} <RelativeTime :at="lastFrameAt" /></span
        >
      </li>
      <li
        v-if="docker"
        class="flex items-center gap-2 border-b border-border px-3 py-3 text-kira-md"
        data-testid="activity-docker"
        :data-state="docker.ok ? 'ok' : 'unavailable'"
      >
        <span class="flex-1">{{ t('common.product.docker.label') }}</span>
        <StatusDot :tone="docker.ok ? 'ok' : 'error'" />
        <span>{{ docker.text }}</span>
      </li>
      <li
        v-for="repo in model?.repoStates ?? []"
        :key="repo.codeRepoId"
        class="flex items-center gap-2 border-b border-border px-3 py-3 text-kira-md"
        data-testid="activity-repo"
      >
        <span class="min-w-0 flex-1 truncate">{{ repoLabel(repo.codeRepoId) }}</span>
        <span class="text-kira-sm text-subtle">{{
          repo.updating ? t('mobile.activity.repos.updating.status') : fetchedText(repo.lastFetchAt)
        }}</span>
      </li>
    </ul>
  </section>
</template>
