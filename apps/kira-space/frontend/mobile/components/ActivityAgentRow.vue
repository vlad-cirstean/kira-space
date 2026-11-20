<script setup lang="ts">
import AdeActivityIcon from '@ade/AdeActivityIcon.vue';
import { activityLabel } from '@ade/activity';
import type { SessionView } from '@ade/sessions/sessionView';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { formatRelative } from '@theme/format';
import { t } from '@theme/i18n';
import { computed } from 'vue';

// One agent on the Activity tab: activity glyph, task, where / what / how long ago, and a link to
// its terminal when the phone may control it.
const props = defineProps<{
  view: SessionView;
  title: string;
  nowMs: number;
  canAttach: boolean;
}>();

const attach = computed(() => props.canAttach && props.view.session.mode === 'tui');
</script>

<template>
  <li
    class="flex items-center gap-3 border-b border-border px-3 py-3"
    data-testid="activity-agent"
    :data-session-id="view.session.id"
    :data-kind="view.kind"
  >
    <AdeActivityIcon :kind="view.kind" :size="14" bare class="w-5" />
    <span class="flex min-w-0 flex-1 flex-col gap-0.5">
      <span class="truncate text-kira-md" data-testid="activity-agent-title">{{ title }}</span>
      <span class="truncate text-kira-sm text-muted-foreground" data-testid="activity-agent-meta">{{
        t('space.statusBar.agents.rowMeta.label', {
          where: view.tabName,
          activity: activityLabel(view.kind),
          ago: formatRelative(view.session.lastActiveAt, nowMs),
        })
      }}</span>
    </span>
    <Badge>{{ view.badge }}</Badge>
    <Button v-if="attach" size="touch" as-child variant="dialog" class="no-underline">
      <RouterLink
        :to="{ name: 'terminal', params: { sessionId: view.session.id } }"
        data-testid="activity-terminal"
      >
        {{ t('common.module.terminal.label') }}
      </RouterLink>
    </Button>
  </li>
</template>
