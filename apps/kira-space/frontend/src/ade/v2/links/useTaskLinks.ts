import { useTaskAction } from '@workbench/behaviour/asyncAction';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed, type MaybeRefOrGetter, toValue } from 'vue';
import { control } from '../../../bridge/control';
import { issueCategoryTone } from '../artifacts/status';
import { taskPatch } from '../board/panelFacts';
import {
  useAddLink,
  useDismissFoundLink,
  useEditLink,
  useRemoveArtifact,
  useTaskArtifacts,
  useTaskIssue,
  useUpdateTask,
} from '../queries';
import { useChangeRequests } from '../readQueries';
import { useMarkTrackedSeen } from '../updates/queries';
import { useTaskUpdates } from '../updates/useTaskUpdates';
import type { Artifact, ArtifactGap, Task } from '../wire';
import { type LinkCategory, type LinkItem, linkGroups } from './linkGroups';

export interface LinkGroup {
  id: LinkCategory;
  items: LinkItem[];
}

/** The task's links as category groups (non-empty only) plus every change to them. */
export function useTaskLinks(
  task: MaybeRefOrGetter<Task>,
  opts: { branches?: MaybeRefOrGetter<boolean> } = {},
) {
  const query = useTaskArtifacts(() => toValue(task).id);
  const crs = useChangeRequests();
  const update = useUpdateTask();
  const addLink = useAddLink();
  const editLink = useEditLink();
  const dismissFoundLink = useDismissFoundLink();
  const removeArtifact = useRemoveArtifact();
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

  // Board change requests of the task's own branches: stored rows with these URLs are its own PRs.
  const ownUrls = computed(() =>
    toValue(task)
      .branchIds.map((id) => crs.data.value?.branches[id]?.url ?? '')
      .filter(Boolean),
  );

  const groups = computed<LinkGroup[]>(() =>
    linkGroups(toValue(task), query.data.value?.artifacts ?? [], {
      branches: toValue(opts.branches) ?? false,
      ownUrls: ownUrls.value,
    })
      .filter((g) => g.items.length > 0)
      .map((g) => ({
        ...g,
        items: g.items.map((item) => {
          if (item.origin !== 'jira') return item;
          if (issue.isPending.value) return { ...item, pending: true };
          const i = issue.data.value;
          if (!i?.title) return item;
          return {
            ...item,
            status: i.status || undefined,
            tone: i.status ? issueCategoryTone(i.statusCategory) : undefined,
          };
        }),
      })),
  );
  // The tracker's reason comes from the issue query, not from Artifacts (same rule as the card's issue line).
  const gaps = computed<ArtifactGap[]>(() => {
    const i = issue.data.value;
    const tracker: ArtifactGap[] =
      i && i.reason !== 'noTracker' && i.reason
        ? [{ source: i.provider, repo: '', reason: i.reason }]
        : [];
    return [...(query.data.value?.gaps ?? []), ...tracker];
  });
  const surface = useTaskAction();

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
    if (item.origin === 'detected' || item.origin === 'found') return;
    await editLink.mutateAsync({
      taskId,
      slot: item.origin,
      id: item.artifactId ?? '',
      text: value,
      kind: '',
    });
  }

  async function remove(item: LinkItem): Promise<void> {
    await surface.run(() => commit(item, ''));
  }

  async function recategorise(item: LinkItem, kind: Artifact['kind']): Promise<void> {
    if (!item.artifactId || kind === item.kind) return;
    await surface.run(() =>
      editLink.mutateAsync({
        taskId: toValue(task).id,
        slot: 'stored',
        id: item.artifactId ?? '',
        text: '',
        kind,
      }),
    );
  }

  /** Files a found link as a main link, the way a paste would: `title url`, else the url. */
  async function promote(item: LinkItem): Promise<void> {
    const text = item.title && item.title !== item.url ? `${item.title} ${item.url}` : item.url;
    await surface.run(() => addLink.mutateAsync({ taskId: toValue(task).id, text }));
  }

  async function dismiss(item: LinkItem): Promise<void> {
    await surface.run(() =>
      dismissFoundLink.mutateAsync({ taskId: toValue(task).id, url: item.url }),
    );
  }

  const add = (line: string) => addLink.mutateAsync({ taskId: toValue(task).id, text: line });

  /**
   * Opens a link in the browser. Returns false when it has no url, so the caller shows the card.
   * A Jira link marks the task's issue update seen.
   */
  function openLink(item: LinkItem): boolean {
    if (item.origin === 'jira') {
      const taskId = toValue(task).id;
      if (pending(taskId, 'issue')) markSeen.mutate({ taskId, itemKeys: ['issue'] });
    }
    if (!item.url) return false;
    fireAndForget(control.openExternal(item.url), 'ade.links');
    return true;
  }

  return {
    query,
    groups,
    gaps,
    error: surface.error,
    add,
    commit,
    remove,
    recategorise,
    promote,
    dismiss,
    openLink,
  };
}

export type TaskLinks = ReturnType<typeof useTaskLinks>;
