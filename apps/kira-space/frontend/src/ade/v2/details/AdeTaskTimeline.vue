<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@theme/components/ui/collapsible';
import { t } from '@theme/i18n';
import { formatRelative } from '@workbench/behaviour/format';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed } from 'vue';
import { control } from '../../../bridge/control';
import AdeRow from '../panel/AdeRow.vue';
import { ADE_ROW, ADE_ROW_BODY, ADE_ROW_LEAD, rowIndent } from '../panel/adeRow';
import { useHandovers, useTaskArtifacts } from '../queries';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import type { Task } from '../wire';
import AdeOriginSession from './AdeOriginSession.vue';
import { type TimelineBranch, taskTimeline } from './taskTimeline';

// What happened to the task, oldest first. Closed until asked for; the choice is kept per task.
const props = defineProps<{
  task: Task;
  branches: readonly TimelineBranch[];
  archived?: boolean;
}>();
const emit = defineEmits<{ openSession: [sessionId: string] }>();

const tabs = useAdePanelTabsStore();
const open = computed({
  get: () => tabs.timelineOpen(props.task.id),
  set: (v) => tabs.setTimelineOpen(props.task.id, v),
});
const artifacts = useTaskArtifacts(() => props.task.id);
const handovers = useHandovers(() => props.task.id);
const entries = computed(() =>
  taskTimeline(
    props.task,
    props.branches,
    artifacts.data.value?.artifacts ?? [],
    handovers.data.value?.handovers ?? [],
  ),
);

function openLink(e: MouseEvent, url: string): void {
  e.preventDefault();
  fireAndForget(control.openExternal(url), 'ade.timeline');
}
</script>

<template>
  <Collapsible v-model:open="open" class="flex flex-col">
    <CollapsibleTrigger
      :class="[ADE_ROW, 'w-full text-left']"
      :style="rowIndent(0)"
      data-testid="ade-details-timeline-toggle"
    >
      <span :class="ADE_ROW_LEAD"
        ><CodiconIcon :name="open ? 'chevron-down' : 'chevron-right'" :size="13" /></span
      >
      <span :class="[ADE_ROW_BODY, 'text-kira-sm text-muted-foreground']">{{
        t('space.ade.details.timeline.header.label', { count: entries.length })
      }}</span>
    </CollapsibleTrigger>
    <CollapsibleContent>
      <ol class="m-0 flex list-none flex-col p-0" data-testid="ade-details-timeline">
        <AdeRow
          v-for="e in entries"
          :key="e.key"
          as="li"
          :label="formatRelative(e.at)"
          data-testid="ade-details-timeline-entry"
          :data-kind="e.kind"
        >
          <a
            v-if="e.url"
            :href="e.url"
            class="min-w-0 truncate text-info"
            @click="openLink($event, e.url)"
            >{{
              e.label
            }}</a
          >
          <span v-else class="min-w-0 truncate">{{ e.label }}</span>
          <template #actions>
            <AdeOriginSession
              :task-id="task.id"
              :session-id="e.sessionId"
              :archived="archived"
              @open="emit('openSession', $event)"
            />
          </template>
        </AdeRow>
      </ol>
    </CollapsibleContent>
  </Collapsible>
</template>
