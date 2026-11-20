<script setup lang="ts">
import StatusDot from '@theme/components/StatusDot.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { useApprovals } from '@workbench/approvals/approvalsQueries';
import { computed } from 'vue';
import { useAgentsModel } from './state/useAgentsModel';
import { runNeedsYou, usePhoneRuns } from './state/usePhoneStatus';

// Need You, Later, Plan and Activity. Plain links: each tab is its own route, so back and reload work.
const { model, nowMs } = useAgentsModel();
const runs = usePhoneRuns();
const { data: approvals } = useApprovals();
const needsBadge = computed(() => (model.value?.needs.badge ?? 0) + (approvals.value?.length ?? 0));
// Activity dot: pulses green while an agent works; amber when a session finished unseen or a
// script run wants you.
const working = computed(() => model.value?.agents.summary.byKind.working ?? 0);
const activityAlert = computed(
  () =>
    (model.value?.agents.summary.byKind.unseen ?? 0) > 0 ||
    (runs.data.value ?? []).some((r) => runNeedsYou(r, nowMs.value)),
);
const TABS = [
  { to: '/needs', label: 'space.common.needYou.label', id: 'needs' },
  { to: '/later', label: 'common.later.label', id: 'later' },
  { to: '/plan', label: 'space.common.plan.label', id: 'plan' },
  { to: '/activity', label: 'mobile.tabs.activity.label', id: 'activity' },
] as const;
</script>

<template>
  <nav
    class="flex border-t border-border bg-chrome pb-[env(safe-area-inset-bottom)]"
    :aria-label="t('mobile.tabs.nav.aria')"
  >
    <RouterLink
      v-for="tab in TABS"
      :key="tab.id"
      :to="tab.to"
      class="flex min-h-12 flex-1 items-center justify-center gap-1.5 text-kira-md text-muted-foreground no-underline aria-[current=page]:font-medium aria-[current=page]:text-fg"
      :data-testid="`tab-${tab.id}`"
    >
      {{ t(tab.label) }}
      <Badge
        v-if="tab.id === 'needs' && needsBadge > 0"
        variant="count"
        data-testid="tab-needs-badge"
        >{{
          needsBadge
        }}</Badge
      >
      <StatusDot
        v-if="tab.id === 'activity' && (working > 0 || activityAlert)"
        :tone="activityAlert ? 'warn' : 'ok'"
        :pulse="working > 0"
        :label="t('space.ade.allSessions.summaryWorking.label', { count: working })"
        data-testid="tab-activity-dot"
      />
    </RouterLink>
  </nav>
</template>
