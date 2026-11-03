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
import { useHandovers, useTaskArtifacts } from '../queries';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import type { Task } from '../wire';
import { type TimelineBranch, taskTimeline } from './taskTimeline';

// What happened to the task, oldest first. Closed until asked for; the choice is kept per task.
const props = defineProps<{ task: Task; branches: readonly TimelineBranch[] }>();

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
  <Collapsible v-model:open="open" class="flex flex-col gap-1">
    <CollapsibleTrigger
      class="flex items-center gap-1 text-left text-kira-sm text-muted-foreground"
      data-testid="ade-details-timeline-toggle"
    >
      <CodiconIcon :name="open ? 'chevron-down' : 'chevron-right'" :size="12" />
      {{ t('space.ade.details.timeline.header.label', { count: entries.length }) }}
    </CollapsibleTrigger>
    <CollapsibleContent>
      <ol class="m-0 flex list-none flex-col gap-0.5 p-0" data-testid="ade-details-timeline">
        <li
          v-for="e in entries"
          :key="e.key"
          class="flex min-h-row min-w-0 items-center gap-2 text-kira-md"
          data-testid="ade-details-timeline-entry"
          :data-kind="e.kind"
        >
          <span class="w-20 shrink-0 text-kira-sm text-subtle">{{ formatRelative(e.at) }}</span>
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
          <slot name="trailing" :entry="e" />
        </li>
      </ol>
    </CollapsibleContent>
  </Collapsible>
</template>
