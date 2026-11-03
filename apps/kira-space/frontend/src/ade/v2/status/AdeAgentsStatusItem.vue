<script setup lang="ts">
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipDetails from '@theme/components/TooltipDetails.vue';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import StatusBarItem from '@workbench/components/StatusBarItem.vue';
import { computed, ref } from 'vue';
import { useAgentSessionsStore } from '../../state';
import { ACTIVITY_RANK, type ActivityKind, activityLabel, withTuiActivity } from '../activity';
import { useSessions } from '../readQueries';
import AdeAgentsStatusList from './AdeAgentsStatusList.vue';
import { agentsSummary } from './agentsSummary';

// Status bar count of running ADE agents (every mode and purpose), hidden at zero. A click opens a
// popover listing them; the list and its plan model load only while it is open. A pulsing dot marks
// agents mid-turn (working).
const sessionsQuery = useSessions();
const agents = useAgentSessionsStore();
const open = ref(false);

const summary = computed(() =>
  agentsSummary(
    withTuiActivity(
      (sessionsQuery.data.value?.sessions ?? []).filter((s) => s.state === 'running'),
      agents.activity,
    ),
  ),
);

const label = computed(() => {
  const { total, byKind } = summary.value;
  const text = t('space.statusBar.agents.label', { count: total });
  return byKind.input > 0
    ? `${text} · ${t('space.ade.allSessions.summaryInput.label', { count: byKind.input })}`
    : text;
});

const rows = computed(() =>
  (Object.keys(summary.value.byKind) as ActivityKind[])
    .filter((k) => k !== 'stopped' && summary.value.byKind[k] > 0)
    .sort((a, b) => ACTIVITY_RANK[a] - ACTIVITY_RANK[b])
    .map((k) => ({ label: activityLabel(k), value: String(summary.value.byKind[k]) })),
);
</script>

<template>
  <Popover v-if="summary.total > 0" v-model:open="open">
    <PopoverTrigger as-child>
      <StatusBarItem
        icon="robot"
        :tone="summary.byKind.input > 0 ? 'warn' : undefined"
        data-testid="status-agents"
        :data-kind="summary.top"
      >
        {{ label }}
        <StatusDot
          v-if="summary.byKind.working > 0"
          tone="ok"
          pulse
          :label="
            t('space.ade.allSessions.summaryWorking.label', { count: summary.byKind.working })
          "
          data-testid="status-agents-working"
        />
        <template #tip>
          <TooltipDetails
            :title="t('space.statusBar.agents.tooltip')"
            :rows="rows"
            :hint="t('space.statusBar.agents.list.hint')"
          />
        </template>
      </StatusBarItem>
    </PopoverTrigger>
    <PopoverContent side="top" align="end" class="w-96 p-1" data-testid="status-agents-popover">
      <AdeAgentsStatusList @close="open = false" />
    </PopoverContent>
  </Popover>
</template>
