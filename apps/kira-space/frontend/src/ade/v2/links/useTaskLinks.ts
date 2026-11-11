import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { logWarn } from '@workbench/bridge/log';
import { computed, type MaybeRefOrGetter, ref, toValue } from 'vue';
import { control } from '../../../bridge/control';
import { useOpenChangeRequest, useOpenHostView } from '../../../hostview/queries';
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
import { useMarkTrackedSeen } from '../updates/queries';
import { useTaskUpdates } from '../updates/useTaskUpdates';
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
  const openRef = useOpenHostView();
  const { pending } = useTaskUpdates();
  const markSeen = useMarkTrackedSeen();
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

  /**
   * Opens a link: in the host view where one exists (change request or issue), else in the browser;
   * `external` always the browser. Returns false when nothing can open (no url, no in-app ref), so the
   * caller shows the card. `onRefused` runs when the view refuses a urlless ref (no browser fallback).
   */
  function openLink(
    item: LinkItem,
    opts: { external?: boolean; onRefused?: () => void } = {},
  ): boolean {
    if (opts.external) {
      if (!item.url) return false;
      fireAndForget(control.openExternal(item.url), 'ade.links');
      return true;
    }
    if (item.origin === 'jira') {
      const taskId = toValue(task).id;
      if (pending(taskId, 'issue')) markSeen.mutate({ taskId, itemKeys: ['issue'] });
    }
    if ((item.kind === 'changeRequest' || item.kind === 'issue') && item.url) {
      fireAndForget(openView.mutateAsync(item.url), 'hostView.open');
      return true;
    }
    if (item.kind === 'issue' && item.ref) {
      openRef.mutateAsync(item.ref).catch((err: unknown) => {
        logWarn('hostView.open', `no view for ${item.ref}`, err);
        opts.onRefused?.();
      });
      return true;
    }
    if (item.url) {
      fireAndForget(control.openExternal(item.url), 'ade.links');
      return true;
    }
    return false;
  }

  return { query, groups, gaps, error, add, commit, remove, recategorise, openLink };
}

export type TaskLinks = ReturnType<typeof useTaskLinks>;
