<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { t } from '@theme/i18n';
import type { MessageKey } from '@theme/i18n/types';
import { rowMeta } from '@theme/lib/rowVariants';
import { errorText } from '@workbench/bridge/codedError';
import QueryState from '@workbench/components/QueryState.vue';
import { computed, ref } from 'vue';
import { gapText } from '../artifacts/gapText';
import { taskPatch } from '../board/panelFacts';
import { type LinkCategory, type LinkItem, linkGroups } from '../links/linkGroups';
import AdeRow from '../panel/AdeRow.vue';
import AdeTaskIssueLine from '../panel/AdeTaskIssueLine.vue';
import {
  useAddLink,
  useEditLink,
  useRemoveArtifact,
  useTaskArtifacts,
  useUpdateTask,
} from '../queries';
import type { Artifact, Task } from '../wire';
import AdeDetailsLink from './AdeDetailsLink.vue';
import AdeLinkAddLine from './AdeLinkAddLine.vue';
import AdeOriginSession from './AdeOriginSession.vue';

// The task's links grouped by kind: Issues, Change requests, Misc (empty groups hidden). One row
// edits at a time; the add line files typed or pasted text, the backend picks the kind.
const props = defineProps<{ task: Task; readonly?: boolean }>();
const emit = defineEmits<{ openSession: [sessionId: string] }>();

const GROUP_LABEL = {
  issue: 'space.settings.updates.issue.title',
  changeRequest: 'space.ade.details.group.changeRequest.label',
  misc: 'space.ade.details.group.misc.label',
} as const satisfies Record<LinkCategory, MessageKey>;

const query = useTaskArtifacts(() => props.task.id);
const update = useUpdateTask();
const addLink = useAddLink();
const editLink = useEditLink();
const removeArtifact = useRemoveArtifact();

const groups = computed(() =>
  linkGroups(props.task, query.data.value?.artifacts ?? []).filter((g) => g.items.length > 0),
);
const gaps = computed(() => query.data.value?.gaps ?? []);
const editingKey = ref<string | null>(null);
const removeError = ref('');

/** Saves one row's edit; an empty text removes it. A failure throws, so the edit stays open. */
async function commit(item: LinkItem, text: string): Promise<void> {
  const taskId = props.task.id;
  const value = text.trim();
  if (!value) {
    if (item.origin === 'jira') {
      await update.mutateAsync({ taskId, patch: taskPatch({ clearJira: true }) });
    } else if (item.origin === 'host') {
      await update.mutateAsync({ taskId, patch: taskPatch({ hostUrl: '' }) });
    } else if (item.artifactId) {
      await removeArtifact.mutateAsync({ taskId, id: item.artifactId });
    }
    return;
  }
  if (item.origin === 'detected') return;
  await editLink.mutateAsync({
    taskId,
    slot: item.origin,
    id: item.artifactId ?? '',
    text: value,
    kind: '',
  });
}

async function remove(item: LinkItem): Promise<void> {
  removeError.value = '';
  try {
    await commit(item, '');
  } catch (err) {
    removeError.value = errorText(err);
  }
}

async function recategorise(item: LinkItem, kind: Artifact['kind']): Promise<void> {
  if (!item.artifactId || kind === item.kind) return;
  removeError.value = '';
  try {
    await editLink.mutateAsync({
      taskId: props.task.id,
      slot: 'stored',
      id: item.artifactId,
      text: '',
      kind,
    });
  } catch (err) {
    removeError.value = errorText(err);
  }
}

const add = (line: string) => addLink.mutateAsync({ taskId: props.task.id, text: line });
</script>

<template>
  <div class="flex flex-col gap-1" data-testid="ade-details-links">
    <div class="flex items-center gap-1 text-kira-sm text-muted-foreground">
      <span class="flex-1" data-testid="ade-details-links-title">{{
        t('space.ade.details.links.title')
      }}</span>
    </div>
    <QueryState :query="query">
      <Alert
        v-for="g in gaps"
        :key="`${g.source}:${g.repo}:${g.reason ?? ''}`"
        variant="warn"
        data-testid="ade-artifact-gap"
      >
        <AlertDescription>{{ gapText(g) }}</AlertDescription>
      </Alert>
      <AdeRow v-if="groups.length === 0" label="" data-testid="ade-details-empty">
        <span :class="rowMeta">{{ t('space.ade.details.empty.body') }}</span>
      </AdeRow>
      <ul
        v-for="group in groups"
        :key="group.id"
        class="m-0 flex list-none flex-col p-0"
        data-testid="ade-details-group"
        :data-group="group.id"
      >
        <AdeDetailsLink
          v-for="(item, i) in group.items"
          :key="item.key"
          :item="item"
          :group-label="i === 0 ? t(GROUP_LABEL[group.id]) : ''"
          :editing="editingKey === item.key"
          :readonly="readonly"
          :commit="commit"
          @edit="editingKey = item.key"
          @done="editingKey = null"
          @remove="remove(item)"
          @kind="recategorise(item, $event)"
        >
          <template #trailing>
            <AdeOriginSession
              :task-id="task.id"
              :session-id="item.sessionId"
              :archived="readonly"
              @open="emit('openSession', $event)"
            />
          </template>
          <template v-if="item.origin === 'jira'" #extra>
            <AdeTaskIssueLine :task-id="task.id" :link="`${item.label} ${item.url}`" />
          </template>
        </AdeDetailsLink>
      </ul>
      <ul v-if="!readonly" class="m-0 flex list-none flex-col p-0">
        <AdeLinkAddLine :add="add" />
      </ul>
      <Alert v-if="removeError" variant="error" data-testid="ade-details-link-error"
        ><AlertDescription>{{ removeError }}</AlertDescription></Alert
      >
    </QueryState>
  </div>
</template>
