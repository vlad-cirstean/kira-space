import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed, type MaybeRefOrGetter, ref, toValue } from 'vue';
import { control } from '../../../bridge/control';
import { useOpenChangeRequest } from '../../../hostview/queries';
import type { Tone } from '../board/actions';
import { taskPatch } from '../board/panelFacts';
import {
  useAddLink,
  useEditLink,
  useRemoveArtifact,
  useTaskArtifacts,
  useTaskIssue,
  useUpdateTask,
} from '../queries';
import type { Artifact, Task } from '../wire';
import { type LinkCategory, type LinkItem, linkGroups } from './linkGroups';

export interface LinkGroup {
  id: LinkCategory;
  items: LinkItem[];
}

const CATEGORY_TONE: Record<string, Tone> = { new: 'grey', indeterminate: 'blue', done: 'green' };

/** The task's links as category groups (non-empty only) plus every change to them. */
export function useTaskLinks(
  task: MaybeRefOrGetter<Task>,
  opts: { branches?: MaybeRefOrGetter<boolean> } = {},
) {
  const query = useTaskArtifacts(() => toValue(task).id);
  const update = useUpdateTask();
  const addLink = useAddLink();
  const editLink = useEditLink();
  const removeArtifact = useRemoveArtifact();
  const openView = useOpenChangeRequest();
  // Same query key as the Jira pill card's issue line: one call serves both.
  const issue = useTaskIssue(
    () => toValue(task).id,
    () => {
      const jira = toValue(task).jira;
      return jira ? `${jira.key} ${jira.url}` : '';
    },
    () => Boolean(toValue(task).jira),
  );

  const groups = computed<LinkGroup[]>(() =>
    linkGroups(toValue(task), query.data.value?.artifacts ?? [], {
      branches: toValue(opts.branches) ?? false,
    })
      .filter((g) => g.items.length > 0)
      .map((g) => ({
        ...g,
        items: g.items.map((item) => {
          const i = issue.data.value;
          if (item.origin !== 'jira' || !i?.title) return item;
          return {
            ...item,
            status: i.status || undefined,
            tone: i.status ? (CATEGORY_TONE[i.statusCategory] ?? 'grey') : undefined,
          };
        }),
      })),
  );
  const gaps = computed(() => query.data.value?.gaps ?? []);
  const error = ref('');

  /** Saves one link's edit; an empty text removes it. A failure throws, so the editor stays open. */
  async function commit(item: LinkItem, text: string): Promise<void> {
    const taskId = toValue(task).id;
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
    error.value = '';
    try {
      await commit(item, '');
    } catch (err) {
      error.value = errorText(err);
    }
  }

  async function recategorise(item: LinkItem, kind: Artifact['kind']): Promise<void> {
    if (!item.artifactId || kind === item.kind) return;
    error.value = '';
    try {
      await editLink.mutateAsync({
        taskId: toValue(task).id,
        slot: 'stored',
        id: item.artifactId,
        text: '',
        kind,
      });
    } catch (err) {
      error.value = errorText(err);
    }
  }

  const add = (line: string) => addLink.mutateAsync({ taskId: toValue(task).id, text: line });

  /** A change request opens in the in-app host view (with its browser fallback); the rest in the browser. */
  function openLink(item: LinkItem): void {
    if (!item.url) return;
    if (item.kind === 'changeRequest') {
      fireAndForget(openView.mutateAsync(item.url), 'hostView.open');
    } else {
      fireAndForget(control.openExternal(item.url), 'ade.details');
    }
  }

  return { query, groups, gaps, error, add, commit, remove, recategorise, openLink };
}

export type TaskLinks = ReturnType<typeof useTaskLinks>;
