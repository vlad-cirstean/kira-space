<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { errorText, toCodedError } from '@workbench/bridge/codedError';
import QueryState from '@workbench/components/QueryState.vue';
import { computed, ref } from 'vue';
import { gapText } from '../artifacts/gapText';
import { taskPatch } from '../board/panelFacts';
import { parseJira } from '../jira';
import AdeTaskIssueLine from '../panel/AdeTaskIssueLine.vue';
import { type LinkPatch, useLinkFields } from '../panel/useLinkFields';
import {
  useAddArtifact,
  useRemoveArtifact,
  useTaskArtifacts,
  useUpdateArtifact,
  useUpdateTask,
} from '../queries';
import type { Task } from '../wire';
import AdeDetailsLink from './AdeDetailsLink.vue';
import { type LinkItem, linkGroups } from './linkGroups';

// The task's links in two groups: Main (Jira, host link, detected change requests) and Related
// (every other artifact). One row edits at a time; the paste line adds links.
const props = defineProps<{ task: Task; readonly?: boolean }>();

const query = useTaskArtifacts(() => props.task.id);
const update = useUpdateTask();
const addArtifact = useAddArtifact();
const updateArtifact = useUpdateArtifact();
const removeArtifact = useRemoveArtifact();

const groups = computed(() => linkGroups(props.task, query.data.value?.artifacts ?? []));
const gaps = computed(() => query.data.value?.gaps ?? []);
const editingKey = ref<string | null>(null);
const removeError = ref('');
const pasteText = ref('');
const pasteError = ref('');

async function write(patch: LinkPatch): Promise<string | null> {
  try {
    await update.mutateAsync({ taskId: props.task.id, patch: taskPatch(patch) });
    return null;
  } catch (err) {
    return errorText(err);
  }
}
const links = useLinkFields(() => props.task.hostLink, write);

function fail(message: string | null): void {
  if (message) throw new Error(message);
}

/** Saves one row's edit; an empty text removes it. A failure throws, so the edit stays open. */
async function commit(item: LinkItem, text: string): Promise<void> {
  const value = text.trim();
  if (item.origin === 'jira') {
    if (value) {
      await links.saveJira(value);
      fail(links.jiraError.value);
    } else fail(await links.clearJira());
  } else if (item.origin === 'host') {
    if (value) {
      await links.saveHost(value);
      fail(links.hostError.value);
    } else fail(await links.clearHost());
  } else if (item.artifactId) {
    const args = { taskId: props.task.id, id: item.artifactId };
    if (value) await updateArtifact.mutateAsync({ ...args, title: value });
    else await removeArtifact.mutateAsync(args);
  }
}

async function remove(item: LinkItem): Promise<void> {
  removeError.value = '';
  try {
    await commit(item, '');
  } catch (err) {
    removeError.value = errorText(err);
  }
}

const isUrl = (raw: string): boolean => {
  try {
    const u = new URL(raw);
    return u.protocol === 'http:' || u.protocol === 'https:';
  } catch {
    return false;
  }
};

/** One pasted value: the task's empty Jira slot, then its empty host slot (the backend says whether
 *  a provider owns the link), else a stored artifact the backend names and sorts. */
async function addValue(raw: string): Promise<void> {
  const taskId = props.task.id;
  const jira = parseJira(raw);
  if (jira.key && !props.task.jira) {
    await update.mutateAsync({ taskId, patch: taskPatch({ jira }) });
    return;
  }
  if (jira.key && props.task.jira?.key !== jira.key && !isUrl(raw)) {
    await addArtifact.mutateAsync({
      taskId,
      kind: 'issue',
      title: jira.key,
      url: '',
      ref: jira.key,
    });
    return;
  }
  if (!isUrl(raw)) throw new Error(t('space.ade.details.paste.notLink.error'));
  if (!props.task.hostUrl) {
    try {
      await update.mutateAsync({ taskId, patch: taskPatch({ hostUrl: raw }) });
      return;
    } catch (err) {
      if (toCodedError(err).code !== 'E_BAD_REQUEST') throw err;
    }
  }
  if (jira.key && props.task.jira?.key !== jira.key) {
    await addArtifact.mutateAsync({
      taskId,
      kind: 'issue',
      title: jira.key,
      url: jira.url,
      ref: jira.key,
    });
    return;
  }
  await addArtifact.mutateAsync({ taskId, kind: 'link', title: '', url: raw, ref: '' });
}

async function submit(text: string): Promise<void> {
  const values = text
    .split(/\r?\n/)
    .map((v) => v.trim())
    .filter(Boolean);
  if (values.length === 0) return;
  pasteError.value = '';
  for (const [i, value] of values.entries()) {
    try {
      await addValue(value);
    } catch (err) {
      pasteText.value = values.slice(i).join('\n');
      pasteError.value = errorText(err);
      return;
    }
  }
  pasteText.value = '';
}

function onPaste(e: ClipboardEvent): void {
  const text = e.clipboardData?.getData('text') ?? '';
  e.preventDefault();
  void submit(text);
}
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="ade-details-links">
    <div class="text-kira-sm text-muted-foreground">{{ t('space.ade.details.links.title') }}</div>
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
        v-for="group in [
          { id: 'main', title: t('common.main.label'), items: groups.main },
          { id: 'related', title: t('space.ade.details.related.title'), items: groups.related },
        ]"
        :key="group.id"
        class="flex flex-col gap-1"
        :data-testid="`ade-details-${group.id}`"
      >
        <div class="text-kira-sm text-subtle">{{ group.title }}</div>
        <div
          v-if="group.items.length === 0"
          class="text-kira-sm text-subtle"
          data-testid="ade-details-empty"
          >{{
            t('space.ade.details.empty.body')
          }}</div
        >
        <ul v-else class="m-0 flex list-none flex-col gap-0.5 p-0">
          <template v-for="item in group.items" :key="item.key">
            <AdeDetailsLink
              :item="item"
              :editing="editingKey === item.key"
              :readonly="readonly"
              :commit="commit"
              @edit="editingKey = item.key"
              @done="editingKey = null"
              @remove="remove(item)"
            />
            <li v-if="item.origin === 'jira'" class="min-w-0 pl-1">
              <AdeTaskIssueLine :task-id="task.id" :link="`${item.label} ${item.url}`" />
            </li>
          </template>
        </ul>
      </div>
      <Alert v-if="removeError" variant="error" data-testid="ade-details-link-error"
        ><AlertDescription>{{ removeError }}</AlertDescription></Alert
      >
    </QueryState>
    <template v-if="!readonly">
      <Input
        v-model="pasteText"
        :placeholder="t('space.ade.details.paste.placeholder')"
        :aria-label="t('space.ade.details.paste.aria')"
        size="kira"
        data-testid="ade-details-paste"
        @keydown.enter.prevent="submit(pasteText)"
        @paste="onPaste"
      />
      <Alert v-if="pasteError" variant="error" data-testid="ade-details-paste-error"
        ><AlertDescription>{{ pasteError }}</AlertDescription></Alert
      >
    </template>
  </div>
</template>
