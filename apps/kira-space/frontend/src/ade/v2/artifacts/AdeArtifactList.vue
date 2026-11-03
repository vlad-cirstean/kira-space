<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { type MessageKey, t } from '@theme/i18n';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import QueryState from '@workbench/components/QueryState.vue';
import { computed } from 'vue';
import { control } from '../../../bridge/control';
import AdeChip from '../AdeChip.vue';
import { useTaskArtifacts } from '../queries';
import type { Artifact } from '../wire';
import { gapText } from './gapText';
import { artifactKindLabel, artifactStatusLabel, artifactStatusTone } from './status';

// Everything a task produced or points at: detected rows (links, branches, change requests,
// deployments) mirror git and the host; stored rows are agent or user reports. Read-only: the
// Details tab owns adding, renaming and removing.
const props = defineProps<{ taskId: string; compact?: boolean }>();
const query = useTaskArtifacts(() => props.taskId);

const SOURCE_LABEL: Record<string, MessageKey> = {
  task: 'common.task.label',
  host: 'common.gitHost.label',
  jira: 'common.product.jira.label',
  git: 'common.module.git.label',
  agent: 'memory.common.origin.agent.label',
  user: 'common.picker.you.label',
};

const gaps = computed(() => query.data.value?.gaps ?? []);

function sourceLabel(a: Artifact): string {
  const key = SOURCE_LABEL[a.source];
  return key ? t(key) : a.source;
}
function openLink(e: MouseEvent, url: string): void {
  e.preventDefault();
  fireAndForget(control.openExternal(url), 'ade.artifact');
}
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="ade-artifacts">
    <div v-if="!compact" class="text-kira-sm text-muted-foreground">{{
      t('space.ade.artifacts.title.title')
    }}</div>
    <QueryState :query="query">
      <Alert
        v-for="g in gaps"
        :key="`${g.source}:${g.repo}:${g.reason ?? ''}`"
        variant="warn"
        data-testid="ade-artifact-gap"
      >
        <AlertDescription>{{ gapText(g) }}</AlertDescription>
      </Alert>
      <div
        v-if="!query.data.value?.artifacts.length"
        class="text-kira-sm text-subtle"
        data-testid="ade-artifact-empty"
      >
        {{ t('space.ade.artifacts.empty.body') }}
      </div>
      <ul v-else class="m-0 flex list-none flex-col gap-1 p-0">
        <li
          v-for="a in query.data.value.artifacts"
          :key="a.id || `${a.kind}:${a.url}:${a.ref}:${a.title}:${a.repo}`"
          class="flex min-w-0 items-center gap-2 text-kira-md"
          data-testid="ade-artifact-row"
        >
          <AdeChip :label="artifactKindLabel(a.kind)" tone="grey" wide />
          <a
            v-if="a.url"
            :href="a.url"
            class="min-w-0 truncate text-info"
            data-testid="ade-artifact-title"
            @click="openLink($event, a.url)"
            >{{
              a.title
            }}</a
          >
          <span v-else class="min-w-0 truncate" data-testid="ade-artifact-title">{{
            a.title
          }}</span>
          <span
            v-if="a.ref && a.ref !== a.title"
            class="min-w-0 truncate font-data text-subtle"
            data-testid="ade-artifact-ref"
            >{{
              a.ref
            }}</span
          >
          <span v-if="a.repo" class="shrink-0 text-subtle" data-testid="ade-artifact-repo">{{
            a.repo
          }}</span>
          <AdeChip
            v-if="a.status"
            :label="artifactStatusLabel(a.status)"
            :tone="artifactStatusTone(a.status)"
          />
          <span
            class="ml-auto shrink-0 text-kira-sm text-subtle"
            data-testid="ade-artifact-source"
            >{{
              sourceLabel(a)
            }}</span
          >
        </li>
      </ul>
    </QueryState>
  </div>
</template>
