<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { Switch } from '@theme/components/ui/switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { paneFrame } from '@theme/lib/frames';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { errorText } from '@workbench/bridge/codedError';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { computed, ref, useTemplateRef, watch } from 'vue';
import { control } from '../../../bridge/control';
import RepoFilePane from '../../../repo/RepoFilePane.vue';
import RepoIdentity from '../../../repo/RepoIdentity.vue';
import { useRepoConfigList } from '../../../repo/state/repoConfigQueries';
import {
  useAddFolder,
  useRemoveFolder,
  useRepos,
  useSetFolderHidden,
  useSetFolderWatch,
} from '../../../repo/state/reposQueries';
import { useCodeReposStore } from '../../../state/coderepos';
import { useAdeReposUiStore } from '../state/adeReposUi';

// Repositories page: the repo list, scan folders and unclaimed settings files beside the settings
// of the picked one. Every switch of the picked file goes through the leave guard.
const reposUi = useAdeReposUiStore();
const codeRepos = useCodeReposStore();
const repos = useRepos();
const repoFiles = useRepoConfigList();
const addFolder = useAddFolder();
const watchFolder = useSetFolderWatch();
const hideFolder = useSetFolderHidden();
const removeFolder = useRemoveFolder();

const folders = computed(() => repos.data.value?.folders ?? []);
const configs = computed(() => repos.data.value?.repos ?? []);
const selected = computed(
  () =>
    codeRepos.records.find((r) => r.id === reposUi.selectedRepoId) ?? codeRepos.records[0] ?? null,
);
// Files that configure no listed repo (the path is unregistered, or an earlier file owns it).
const otherFiles = computed(() => (repoFiles.data.value?.files ?? []).filter((f) => !f.repo));
const pickedFile = ref('');
const pickedOther = computed(
  () => otherFiles.value.find((f) => f.file === pickedFile.value) ?? otherFiles.value[0] ?? null,
);
const hasProblem = (id: string) => !!configs.value.find((c) => c.codeRepoId === id)?.problem;
const nickOf = (id: string) => configs.value.find((c) => c.codeRepoId === id)?.nickname ?? '';
const selectedConfig = computed(
  () => configs.value.find((c) => c.codeRepoId === selected.value?.id) ?? null,
);
async function pickOtherFile(file: string): Promise<void> {
  if (pickedOther.value?.file === file) return;
  if (!(await reposUi.leave())) return;
  pickedFile.value = file;
}
const listEl = useTemplateRef<HTMLElement>('listEl');
const activeRepoKey = ref<string | null>(null);
watch(
  () => selected.value?.id ?? null,
  (id) => {
    activeRepoKey.value = id;
  },
  { immediate: true },
);
// The selection follows the arrows (master-detail): each stop shows that repo's settings.
const rowHeight = useRowHeight();
const { virtualizer, virtualItems, totalSize, onScroll, scrollToIndex } = useVirtualRows({
  count: () => codeRepos.records.length,
  rowHeight: () => rowHeight.single.value + 2,
  scrollElement: listEl,
});
const windowRows = computed(() =>
  virtualItems.value.flatMap((item) => {
    const r = codeRepos.records[item.index];
    return r ? [{ item, r }] : [];
  }),
);
function measure(el: unknown): void {
  if (el instanceof Element) virtualizer.value.measureElement(el);
}
const nav = useListNavigation({
  container: listEl,
  items: () => codeRepos.records,
  key: (r) => r.id,
  activeKey: activeRepoKey,
  selectOnMove: true,
  onSelect: (r) => void reposUi.pickRepo(r.id),
  scrollToIndex: (index) => scrollToIndex(index),
  rendered: () => virtualItems.value,
});
const showRepos = computed(() => reposUi.tab === 'repos');
const showFiles = computed(() => reposUi.tab === 'files');
const title = computed(() =>
  showRepos.value
    ? (selected.value?.name ?? t('common.module.repositories.label'))
    : showFiles.value
      ? t('space.repo.settingsFile.otherFiles.label')
      : t('space.ade.repos.scanFolders.label'),
);
const error = ref('');
const note = ref('');

async function run(task: () => Promise<void>): Promise<void> {
  error.value = '';
  note.value = '';
  try {
    await task();
  } catch (err) {
    error.value = errorText(err);
  }
}

const onImport = () => run(async () => void (await codeRepos.importRepoViaDialog()));

const onAddFolder = () =>
  run(async () => {
    const chosen = await control.filesChooseFolder(t('space.ade.repos.addFolder.title'));
    if (chosen.canceled || !chosen.path) return;
    const res = await addFolder.mutateAsync({ path: chosen.path, watch: true });
    note.value = t('space.ade.repos.imported.announce', { count: res.imported.length });
  });

const onWatch = (path: string, watch: boolean) =>
  run(async () => void (await watchFolder.mutateAsync({ path, watch })));

const onRemoveFolder = (path: string) =>
  run(async () => void (await removeFolder.mutateAsync({ path })));

const onHideFolder = (path: string, hidden: boolean) =>
  run(async () => void (await hideFolder.mutateAsync({ path, hidden })));

const onHideRepo = (id: string, hidden: boolean) =>
  run(() => codeRepos.setCodeRepoHidden(id, hidden));

const onRemoveRepo = (id: string, name: string) =>
  run(async () => void (await codeRepos.confirmRemoveCodeRepo(id, name)));

/** Parent directory of the checkout with its trailing separator, the dim half of the head's path. */
function parentDir(root: string): string {
  const trimmed = root.replace(/[\\/]+$/, '');
  const i = Math.max(trimmed.lastIndexOf('/'), trimmed.lastIndexOf('\\'));
  return i >= 0 ? trimmed.slice(0, i + 1) : '';
}

function baseName(path: string): string {
  return (
    path
      .replace(/[\\/]+$/, '')
      .split(/[\\/]/)
      .pop() ?? path
  );
}

function repoRowClass(r: { id: string; hidden: boolean }): string {
  return rowVariants({
    layout: 'nav',
    selected: showRepos.value && selected.value?.id === r.id,
    muted: r.hidden,
  });
}
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 gap-0.5" data-testid="ade-repos">
    <nav :class="cn(paneFrame, 'flex w-75 shrink-0 flex-col')">
      <PanelHeader>
        {{ t('common.module.repositories.label') }}
        <template #actions>
          <Button
            variant="dialog"
            size="kira-lg"
            class="px-1.5"
            data-testid="ade-repos-import"
            @click="onImport"
          >
            <CodiconIcon name="repo" :size="13" />
            {{ t('space.ade.repos.import.label') }}
          </Button>
        </template>
      </PanelHeader>
      <div
        ref="listEl"
        class="min-h-0 flex-1 overflow-y-auto px-1 py-1.5"
        role="listbox"
        :aria-label="t('common.module.repositories.label')"
        data-testid="ade-repos-repos"
        @scroll.passive="onScroll"
      >
        <p
          v-if="codeRepos.records.length === 0"
          class="m-0 px-1.5 py-1 text-kira-md text-muted-foreground"
          >{{
            t('space.ade.repos.empty.empty')
          }}</p
        >
        <div v-else class="relative w-full" :style="{ height: `${totalSize}px` }">
          <div
            v-for="{ item, r } in windowRows"
            :key="r.id"
            :ref="measure"
            :data-index="item.index"
            :class="cn(VIRTUAL_ROW_CLASS, 'pb-px')"
            :style="{ transform: `translateY(${item.start}px)` }"
          >
            <Tooltip>
              <TooltipTrigger as-child>
                <button
                  type="button"
                  role="option"
                  v-bind="nav.rowProps(r.id)"
                  class="relative flex w-full shrink-0 items-center gap-1.5 border-none text-left"
                  :class="repoRowClass(r)"
                  :aria-selected="showRepos && selected?.id === r.id"
                  data-testid="ade-repos-repo"
                  :data-repo-id="r.id"
                  :data-hidden="r.hidden"
                  @click="reposUi.pickRepo(r.id)"
                >
                  <span
                    :class="colorMarkClass('rail', r.color)"
                    data-testid="ade-repos-repo-rail"
                    aria-hidden="true"
                  />
                  <span class="min-w-0 flex-1 truncate">{{ nickOf(r.id) || r.name }}</span>
                  <StatusDot
                    v-if="hasProblem(r.id)"
                    tone="error"
                    :label="t('space.repo.settingsFile.problem.aria')"
                    data-testid="ade-repos-repo-problem"
                  />
                  <CodiconIcon
                    v-if="r.hidden"
                    name="eye-closed"
                    :size="13"
                    class="shrink-0"
                    data-testid="ade-repos-repo-hidden-mark"
                  />
                </button>
              </TooltipTrigger>
              <TooltipContent>{{ r.root }}</TooltipContent>
            </Tooltip>
          </div>
        </div>
      </div>
      <div class="flex flex-col gap-px px-1 py-1.5">
        <button
          type="button"
          class="flex w-full items-center gap-1.5 border-none text-left"
          :class="rowVariants({ layout: 'nav', selected: reposUi.tab === 'folders' })"
          :aria-current="reposUi.tab === 'folders' ? 'page' : undefined"
          data-testid="ade-repos-tab-folders"
          @click="reposUi.setTab('folders')"
        >
          <CodiconIcon name="folder" :size="13" />
          {{ t('space.ade.repos.scanFolders.label') }}
        </button>
        <button
          type="button"
          class="flex w-full items-center gap-1.5 border-none text-left"
          :class="rowVariants({ layout: 'nav', selected: showFiles })"
          :aria-current="showFiles ? 'page' : undefined"
          data-testid="ade-repos-tab-files"
          @click="reposUi.setTab('files')"
        >
          <CodiconIcon name="file" :size="13" />
          <span class="min-w-0 flex-1 truncate">{{
            t('space.repo.settingsFile.otherFiles.label')
          }}</span>
          <span
            v-if="otherFiles.length > 0"
            class="shrink-0 text-kira-sm text-muted-foreground"
            data-testid="ade-repos-files-count"
            >{{
              otherFiles.length
            }}</span
          >
        </button>
      </div>
    </nav>

    <section :class="cn(paneFrame, 'flex min-w-0 flex-1 flex-col')">
      <PanelHeader>{{ title }}</PanelHeader>
      <div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
        <Alert v-if="error" variant="error" strip data-testid="ade-repos-error">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
        <template v-if="showRepos">
          <template v-if="selected">
            <ViewToolbar data-testid="ade-repos-repo-head" :data-repo-id="selected.id">
              <span :class="colorMarkClass('dot', selected.color)" />
              <span class="flex size-4 shrink-0 items-center justify-center text-muted-foreground">
                <CodiconIcon name="repo" :size="13" />
              </span>
              <span class="min-w-0 text-kira-md text-fg" data-testid="ade-repos-selected"
                ><TruncatedText :text="selected.root"
                  ><span v-if="parentDir(selected.root)" class="text-subtle">{{
                    parentDir(selected.root)
                  }}</span>{{ selected.name }}</TruncatedText
                ></span
              >
              <Tooltip v-if="selectedConfig" :disabled="selectedConfig.source === 'added'">
                <TooltipTrigger as-child>
                  <Badge class="shrink-0" data-testid="ade-repos-repo-source">{{
                    selectedConfig.source === 'added'
                      ? t('common.added.status')
                      : baseName(selectedConfig.source)
                  }}</Badge>
                </TooltipTrigger>
                <TooltipContent>{{ selectedConfig.source }}</TooltipContent>
              </Tooltip>
              <Badge
                v-if="selected.hidden"
                variant="warn"
                class="shrink-0"
                data-testid="ade-repos-repo-hidden-badge"
                >{{
                  t('space.common.hidden.status')
                }}</Badge
              >
              <div class="ml-auto flex shrink-0 items-center gap-1">
                <TooltipIconButton
                  :icon="selected.hidden ? 'eye' : 'eye-closed'"
                  :label="
                    selected.hidden
                      ? t('space.ade.repos.show.tooltip')
                      : t('space.ade.repos.hide.tooltip')
                  "
                  :aria-pressed="selected.hidden"
                  data-testid="ade-repos-repo-hide"
                  @click="onHideRepo(selected.id, !selected.hidden)"
                />
                <TooltipIconButton
                  icon="trash"
                  :label="t('space.ade.repos.remove.tooltip')"
                  data-testid="ade-repos-repo-remove"
                  @click="onRemoveRepo(selected.id, selected.name)"
                />
              </div>
            </ViewToolbar>
            <div :class="colorMarkClass('band', selected.color)" />
            <div v-if="selectedConfig" class="m-3 flex flex-col gap-4">
              <RepoIdentity :key="selectedConfig.codeRepoId" :repo="selectedConfig" />
              <RepoFilePane
                :key="selectedConfig.codeRepoId"
                :path="selected.repoId"
                @dirty="(v) => (reposUi.dirty = v)"
              />
            </div>
          </template>
          <p v-else class="m-3 text-muted-foreground">{{
            t('space.ade.repos.configureHint.emptyHint')
          }}</p>
        </template>
        <div v-else-if="showFiles" class="flex flex-col gap-2 p-3" data-testid="ade-repos-files">
          <p class="m-0 text-muted-foreground">{{
            t('space.repo.settingsFile.otherFiles.hint')
          }}</p>
          <p v-if="otherFiles.length === 0" class="m-0 text-muted-foreground">{{
            t('space.repo.settingsFile.otherFiles.empty')
          }}</p>
          <template v-else>
            <div class="flex flex-wrap gap-1">
              <Button
                v-for="f in otherFiles"
                :key="f.file"
                variant="dialog"
                size="kira-lg"
                :aria-pressed="pickedOther?.file === f.file"
                data-testid="ade-repos-file"
                :data-file="f.file"
                @click="pickOtherFile(f.file)"
              >
                <StatusDot
                  v-if="f.problem"
                  tone="error"
                  :label="t('space.repo.settingsFile.problem.aria')"
                />
                {{ f.file }}
              </Button>
            </div>
            <RepoFilePane
              v-if="pickedOther"
              :key="pickedOther.file"
              :path="pickedOther.path"
              :file="pickedOther.file"
              @dirty="(v) => (reposUi.dirty = v)"
            />
          </template>
        </div>
        <div v-else class="flex flex-col gap-2 p-3" data-testid="ade-repos-folders">
          <div class="flex items-center gap-2">
            <p class="m-0 flex-1 text-muted-foreground">{{
              t('space.ade.repos.foldersHint.hint')
            }}</p>
            <Button
              variant="dialog"
              size="kira-lg"
              :pending="addFolder.isPending.value"
              data-testid="ade-repos-add-folder"
              @click="onAddFolder"
            >
              <CodiconIcon name="folder" :size="13" />
              {{ t('space.ade.repos.addFolder.label') }}
            </Button>
          </div>
          <FieldDescription v-if="note" data-testid="ade-repos-note">{{ note }}</FieldDescription>
          <p v-if="folders.length === 0" class="m-0 text-muted-foreground">{{
            t('space.ade.repos.noFolders.empty')
          }}</p>
          <div
            v-for="(f, i) in folders"
            :key="f.path"
            class="flex items-center gap-2 rounded-kira bg-field px-2 py-1"
            data-testid="ade-repos-folder"
            :data-path="f.path"
          >
            <span class="min-w-0 flex-1 font-data"><TruncatedText :text="f.path" /></span>
            <span
              class="shrink-0 text-kira-sm text-muted-foreground"
              data-testid="ade-repos-folder-count"
              >{{
                f.hiddenCount > 0
                  ? t('common.dotPair.label', {
                      first: t('space.ade.repos.folder.repoCount.label', {
                        count: f.repoCount,
                      }),
                      second: t('space.ade.repos.folder.hiddenCount.label', {
                        count: f.hiddenCount,
                      }),
                    })
                  : t('space.ade.repos.folder.repoCount.label', { count: f.repoCount })
              }}</span
            >
            <Tooltip>
              <TooltipTrigger as-child>
                <div class="flex shrink-0 items-center gap-1.5">
                  <Switch
                    :id="`ade-repos-watch-${i}`"
                    :model-value="f.watch"
                    data-testid="ade-repos-folder-watch"
                    @update:model-value="(v: boolean) => onWatch(f.path, v)"
                  />
                  <Label :for="`ade-repos-watch-${i}`" class="text-kira-sm text-muted-foreground">{{
                    t('space.ade.repos.folder.watch.label')
                  }}</Label>
                </div>
              </TooltipTrigger>
              <TooltipContent>{{ t('space.ade.repos.folder.watch.tooltip') }}</TooltipContent>
            </Tooltip>
            <TooltipIconButton
              :icon="f.hidden ? 'eye' : 'eye-closed'"
              :label="
                f.hidden
                  ? t('common.action.showAll.label')
                  : t('space.ade.repos.folder.hideAll.label')
              "
              :aria-pressed="f.hidden"
              data-testid="ade-repos-folder-hide"
              @click="onHideFolder(f.path, !f.hidden)"
            />
            <TooltipIconButton
              icon="close"
              :label="t('space.ade.repos.folder.remove.tooltip')"
              data-testid="ade-repos-folder-remove"
              @click="onRemoveFolder(f.path)"
            />
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
