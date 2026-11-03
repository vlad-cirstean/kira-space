<script setup lang="ts">
import type { FileStat } from '@shared/domain/repo';
import EmptyState from '@theme/components/EmptyState.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, onMounted, onUnmounted, ref, useTemplateRef } from 'vue';
import { gitRepoIdFor } from '../../repo/git/hostHandlers';
import { useFileEditStore } from '../../state/fileEdit';
import type { RepoFileTabRecord } from '../../state/tabDomain';
import { useTabsStore } from '../../state/tabs';
import { noRepositoryMessage, repoIdOfTab } from '../../state/workspace';
import { attachDistillSelection } from './distillSelection';
import { knownRead, registerEditor, rememberRead, unmountEditor } from './editors';
import { FULL_COMMIT_ID, loadFileContent } from './fileContent';
import { loadMonaco, peekModel, repoFileUri, repoRevisionFileUri } from './monaco';
import RepoFileEditBar from './RepoFileEditBar.vue';
import { applyMountReveal, consumeReveal } from './reveal';
import { useFileEdit } from './useFileEdit';
import { type InlineBlameAttachParams, useInlineBlame } from './useInlineBlame';
import { useReadonlyFileViewer } from './useReadonlyFileViewer';

const props = defineProps<{ tab: RepoFileTabRecord }>();
const tabsStore = useTabsStore();
const fileEdit = useFileEditStore();

type ViewState = 'loading' | 'found' | 'binary' | 'tooLarge' | 'missing' | 'error';
const state = ref<ViewState>('loading');
const errorMessage = ref('');

// Only true `markdown` (never `.mdx`, which colors as its own Monaco language) grows a toolbar;
// every other file type stays byte-identical. Opens on Source and restores the choice per tab.
const viewer = useReadonlyFileViewer({
  tabId: props.tab.id,
  path: props.tab.path,
  markdownReading: props.tab.state.markdownReading,
  persistReading: (reading) =>
    tabsStore.patchRepoFileTabState(props.tab.id, { markdownReading: reading }),
  testidPrefix: 'repo-file',
});
// biome-ignore lint/correctness/noUnusedVariables: bound by the template ref="readingPane"
const { container, readingPane, isMarkdown, view, viewOptions, renderedHtml, onViewChange } =
  viewer;

const root = useTemplateRef<HTMLElement>('root');
// Only a worktree tab (not revision-pinned) whose file read as text can be edited.
const canEdit = computed(() => props.tab.state.rev === null && state.value === 'found');
let blameParams: InlineBlameAttachParams | null = null;

function teardownEditor(): void {
  disposeCursorSub?.();
  disposeCursorSub = null;
  disposeDistill?.();
  disposeDistill = null;
  edit.detach();
  viewer.dispose();
  inlineBlame.dispose();
  unmountEditor(props.tab.id);
}

const edit = useFileEdit({
  tabId: props.tab.id,
  path: props.tab.path,
  root,
  repoId: () => repoIdOfTab(props.tab),
  showSource: () => {
    if (viewer.isMarkdown.value && viewer.view.value === 'reading') viewer.onViewChange('source');
  },
  onText: (text) => viewer.setText(text),
  dropBlame: () => inlineBlame.dispose(),
  restoreBlame: () => {
    if (blameParams) inlineBlame.attach(blameParams);
  },
  onGone: () => {
    teardownEditor();
    state.value = 'missing';
  },
});

let disposeCursorSub: (() => void) | null = null;
let disposeDistill: (() => void) | null = null;
const inlineBlame = useInlineBlame();

type ResolvedText = { text: string; stat?: FileStat } | { state: ViewState; message?: string };

// The tab's text: the model's draft mid-edit, the model itself for a fixed commit, otherwise a read
// that the backend skips while the file still matches what the model holds.
async function resolveText(
  repoId: string,
  rev: string | null,
  cachedModel: ReturnType<typeof peekModel>,
): Promise<ResolvedText> {
  // Back from a tab switch mid-edit: the cached model holds the draft, so skip the read that would
  // overwrite it with the disk text.
  const draft = fileEdit.isEditing(props.tab.id) ? cachedModel : undefined;
  if (draft) return { text: draft.getValue() };
  fileEdit.end(props.tab.id);
  // A commit's content never changes: the model from the last mount is current.
  if (cachedModel && rev !== null && FULL_COMMIT_ID.test(rev))
    return { text: cachedModel.getValue() };
  const known = cachedModel ? knownRead(props.tab.id, [cachedModel.getVersionId()]) : null;
  const result = await loadFileContent(
    repoId,
    props.tab.path,
    rev,
    cachedModel && known?.stat
      ? { stat: known.stat, text: () => cachedModel.getValue() }
      : undefined,
  );
  if (result.kind === 'error') return { state: 'error', message: result.message };
  if (result.kind !== 'found') return { state: result.kind };
  return { text: result.text, stat: result.stat };
}

async function mount(): Promise<void> {
  const repoId = repoIdOfTab(props.tab);
  if (!repoId) {
    state.value = 'error';
    errorMessage.value = noRepositoryMessage();
    return;
  }

  const rev = props.tab.state.rev;
  const mod = await loadMonaco();
  const uri =
    rev === null
      ? repoFileUri(mod, repoId, props.tab.path)
      : repoRevisionFileUri(mod, repoId, props.tab.path, rev);
  const cachedModel = peekModel(uri);
  const loaded = await resolveText(repoId, rev, cachedModel);
  if ('state' in loaded) {
    state.value = loaded.state;
    errorMessage.value = loaded.message ?? '';
    return;
  }
  const { text, stat } = loaded;
  // D11: held on a ref (not only handed to the model) so the reading view can render it too.
  viewer.setText(text);

  // Unmounted (tab closed/switched away) while the read/import above was in flight.
  if (!container.value) return;

  const editor = viewer.createEditor(mod, uri, text);
  if (!editor) return;
  const restored = registerEditor(props.tab.id, uri, editor);
  const model = peekModel(uri);
  if (model && stat) rememberRead(props.tab.id, { stat, versionIds: [model.getVersionId()] });
  edit.attach(editor, mod);
  disposeDistill = attachDistillSelection(editor, props.tab.path);

  // Attached after the editor exists, guarded on the setting and on whether this window has a git
  // record for the repository at all (`gitRepoIdFor`'s own "never guessed" contract): a silent
  // no-op either way. Read live so toggling the setting takes effect on the open tab immediately.
  const gitRepoId = gitRepoIdFor(repoId);
  // `blame.line` always blames the working tree; a revision-pinned tab shows different bytes, so
  // its line numbers do not correspond.
  const blameable = gitRepoId !== undefined && rev === null;
  if (blameable && gitRepoId) {
    blameParams = {
      gitRepoId,
      workspaceRepoId: repoId,
      path: props.tab.path,
      editor,
      mod,
    };
    // Edit mode drops blame: its line numbers stop matching once the text changes.
    if (!fileEdit.isEditing(props.tab.id)) inlineBlame.attach(blameParams);
  }

  // A pending reveal (a search result or go-to-definition match that arrived while this tab wasn't
  // mounted) wins over the persisted revealLine.
  applyMountReveal(editor, consumeReveal(props.tab.id), restored, props.tab.state.revealLine);

  // Debounced patch (patchRepoFileTabState's own skipUnchanged) re-persists revealLine as the user
  // scrolls/navigates, so a restored session reopens roughly where it was left.
  const sub = editor.onDidChangeCursorPosition((e) => {
    tabsStore.patchRepoFileTabState(props.tab.id, { revealLine: e.position.lineNumber });
  });
  disposeCursorSub = () => sub.dispose();

  state.value = 'found';
}

onMounted(() => void mount());

// A mere tab switch away (MainView's own `v-if`): the widget disposes, the model (and the
// tabId -> uri mapping) survives in editors.ts until an actual close calls
// TAB_KINDS['repo-file'].dropResources. Remounting re-runs mount(), which reuses the cached model.
onUnmounted(teardownEditor);
</script>

<template>
  <div
    v-if="state === 'loading' || state === 'found'"
    ref="root"
    class="flex flex-col h-full w-full"
  >
    <ViewToolbar v-if="isMarkdown || canEdit" border="none">
      <SecondaryTabs
        v-if="isMarkdown && !edit.editing.value"
        :model-value="view"
        :items="viewOptions"
        data-testid="repo-file-view-toggle"
        @update:model-value="(v) => onViewChange(v as 'source' | 'reading')"
      />
      <RepoFileEditBar
        v-if="canEdit"
        :editing="edit.editing.value"
        :dirty="edit.dirty.value"
        :saving="edit.saving.value"
        :locked="edit.busy.value"
        @edit="edit.enter()"
        @save="edit.save()"
        @done="edit.done()"
      />
    </ViewToolbar>
    <Alert v-if="edit.conflict.value" variant="warn" strip data-testid="repo-file-conflict">
      <AlertDescription>
        {{
          edit.conflict.value === 'gone'
            ? t('space.repo.file.deletedOnDisk.body')
            : t('space.repo.file.changedOnDisk.body')
        }}
      </AlertDescription>
      <AlertAction>
        <Button variant="toolbar" size="kira" data-testid="repo-file-reload" @click="edit.reload()">
          {{ t('common.action.refresh.label') }}
        </Button>
        <Button
          variant="toolbar"
          size="kira"
          data-testid="repo-file-overwrite"
          @click="edit.overwrite()"
        >
          {{ t('space.ade.workflows.conflict.overwrite.label') }}
        </Button>
      </AlertAction>
    </Alert>
    <Alert v-if="edit.errorMessage.value" variant="error" strip data-testid="repo-file-edit-error">
      <AlertDescription>{{ edit.errorMessage.value }}</AlertDescription>
    </Alert>
    <!-- `v-show`, never `v-if` — the editor widget must never be disposed/recreated by the markdown
         toggle, only hidden, so scroll position/selection/find state survive a round trip. -->
    <div
      v-show="!isMarkdown || view === 'source'"
      ref="container"
      class="monaco-host flex-1 min-h-0"
      data-testid="repo-file-editor"
    />
    <!-- D11: `markdown-it`'s `html: false` (markdownReading.ts) escapes any literal HTML tag in
         the source, so this is the one `v-html` in this view that's safe. -->
    <div
      v-if="isMarkdown && view === 'reading'"
      ref="readingPane"
      class="kira-prose flex-1 min-h-0 overflow-auto text-fg bg-bg font-ui text-kira-md leading-relaxed p-4 [&>*]:max-w-prose [&>pre]:max-w-none [&>table]:max-w-none"
      data-testid="repo-file-markdown"
      v-html="renderedHtml"
    />
  </div>
  <EmptyState
    v-else-if="state === 'binary'"
    fill
    icon="file-binary"
    :title="t('space.repo.view.binaryPreview.empty')"
  />
  <EmptyState
    v-else-if="state === 'tooLarge'"
    fill
    icon="warning"
    :title="t('space.repo.view.tooLargePreview.empty')"
  />
  <EmptyState
    v-else-if="state === 'missing'"
    fill
    icon="warning"
    :title="t('space.repo.view.missing.empty')"
  />
  <EmptyState
    v-else
    fill
    icon="warning"
    :title="errorMessage || t('space.repo.view.openFileFailed.error')"
  />
</template>
