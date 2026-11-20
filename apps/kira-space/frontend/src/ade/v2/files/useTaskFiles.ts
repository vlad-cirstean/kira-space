import type { FileListing, FileRead } from '@shared/domain/repo';
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed, type MaybeRefOrGetter, toValue } from 'vue';
import { control } from '../../../bridge/control';
import { useAdeFilesUiStore } from './state/filesUi';

const launchersKey = ['adetask', 'files', 'launchers'] as const;
const filesKey = (branchId: string) => ['adetask', 'files', branchId] as const;

/** The branch worktree's tracked and untracked files. Refetches on mount and on Refresh. */
export function useTaskFilesListing(branchId: MaybeRefOrGetter<string>) {
  return useQuery<FileListing>(() => {
    const id = toValue(branchId);
    return {
      queryKey: [...filesKey(id), 'list'],
      queryFn: () => control.adeTaskFiles({ branchId: id }),
      enabled: id !== '',
    };
  });
}

/** One worktree file. A refetch sends the previous stat; an unchanged answer keeps the prior text. */
export function useTaskFileRead(
  branchId: MaybeRefOrGetter<string>,
  path: MaybeRefOrGetter<string>,
) {
  const queryClient = useQueryClient();
  return useQuery<FileRead>(() => {
    const id = toValue(branchId);
    const p = toValue(path);
    const queryKey = [...filesKey(id), 'read', p];
    return {
      queryKey,
      queryFn: async () => {
        const prev = queryClient.getQueryData<FileRead>(queryKey);
        const ifStat = prev?.kind === 'found' ? prev.stat : undefined;
        const next = await control.adeTaskFileRead(id, p, ifStat);
        return next.kind === 'unchanged' && prev ? prev : next;
      },
      enabled: id !== '' && p !== '',
      // A remount re-reads: the editor is rebuilt each time, so nothing is kept past unmount.
      gcTime: 0,
    };
  });
}

/** Editor registry id of one worktree file's viewer; shared by the viewer and a search reveal. */
export const taskFileTabId = (branchId: string, path: string): string =>
  `ade-file:${branchId}:${path}`;

const REVEAL_LABEL = {
  finder: 'space.ade.files.reveal.finder.label',
  explorer: 'space.ade.files.reveal.explorer.label',
  fileManager: 'space.ade.files.reveal.fileManager.label',
} as const;

/** Reveal in the file manager and Open in VS Code for a branch's worktree: what this build
 *  offers, the two mutations, and the menu entries. A rel path of '' is the worktree root. */
export function useTaskFileLaunch(branchId: MaybeRefOrGetter<string>) {
  const filesUi = useAdeFilesUiStore();
  const launchers = useQuery({
    queryKey: [...launchersKey],
    // Fixed for the process lifetime: the query client default keeps it fresh.
    queryFn: () => control.adeTaskFilesLaunchers(),
  });
  const revealer = computed(() => launchers.data.value?.reveal ?? 'none');
  const canReveal = computed(() => revealer.value !== 'none');
  const canOpenEditor = computed(() => launchers.data.value?.editor === true);
  const onError = (err: unknown) => {
    filesUi.launchError[toValue(branchId)] = errorText(err);
  };
  const onMutate = () => {
    delete filesUi.launchError[toValue(branchId)];
  };
  const reveal = useMutation({
    mutationFn: (path: string) => control.adeTaskFileReveal(toValue(branchId), path),
    onMutate,
    onError,
  });
  const openInEditor = useMutation({
    mutationFn: (path: string) => control.adeTaskFileOpenInEditor(toValue(branchId), path),
    onMutate,
    onError,
  });

  function revealLabel(): string {
    const kind = revealer.value;
    return kind === 'none' ? '' : t(REVEAL_LABEL[kind]);
  }
  const targetOf = (path: string) =>
    t(path === '' ? 'common.worktree.label' : 'space.ade.files.target.file.label');
  function revealTooltip(path: string): string {
    return t('common.dashPair.label', { first: revealLabel(), second: targetOf(path) });
  }
  function editorTooltip(path: string): string {
    return t('common.dashPair.label', {
      first: t('space.ade.files.openInEditor.label'),
      second: targetOf(path),
    });
  }
  /** Context-menu entries for the row at `path`; empty when this build offers neither action. */
  function menuItems(path: string): MenuItem[] {
    const items: MenuItem[] = [];
    if (canReveal.value) {
      items.push({
        type: 'item',
        id: 'reveal',
        label: revealLabel(),
        icon: 'folder-opened',
        run: () => reveal.mutate(path),
      });
    }
    if (canOpenEditor.value) {
      items.push({
        type: 'item',
        id: 'open-in-editor',
        label: t('space.ade.files.openInEditor.label'),
        icon: 'code',
        run: () => openInEditor.mutate(path),
      });
    }
    return items;
  }

  return {
    canReveal,
    canOpenEditor,
    reveal: (path: string) => reveal.mutate(path),
    openInEditor: (path: string) => openInEditor.mutate(path),
    revealTooltip,
    editorTooltip,
    menuItems,
  };
}
