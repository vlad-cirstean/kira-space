<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { dropRepoFileTab, registerEditor } from '../../../views/repo/editors';
import { loadMonaco, taskFileUri } from '../../../views/repo/monaco';
import { applyMountReveal, consumeReveal } from '../../../views/repo/reveal';
import { useReadonlyFileViewer } from '../../../views/repo/useReadonlyFileViewer';
import { useAdeFilesUiStore } from './state/filesUi';
import { taskFileTabId, useTaskFileRead } from './useTaskFiles';

// Read-only Monaco over one worktree file. The parent keys it by path, so one instance shows one
// file; a refetch that changes the text replaces the model's value in place.
const props = defineProps<{ branchId: string; path: string }>();
const filesUi = useAdeFilesUiStore();

const tabId = taskFileTabId(props.branchId, props.path);
const viewer = useReadonlyFileViewer({
  tabId,
  path: props.path,
  markdownReading: filesUi.reading[props.branchId] ?? false,
  persistReading: (reading) => {
    filesUi.reading[props.branchId] = reading;
  },
  testidPrefix: 'ade-files',
});
// biome-ignore lint/correctness/noUnusedVariables: bound by the template ref="readingPane"
const { container, readingPane, isMarkdown, view, viewOptions, renderedHtml, onViewChange } =
  viewer;

const query = useTaskFileRead(
  () => props.branchId,
  () => props.path,
);

type ViewState = 'loading' | 'found' | 'binary' | 'tooLarge' | 'missing' | 'error';
const state = computed<ViewState>(() => {
  if (query.isError.value) return 'error';
  const data = query.data.value;
  if (!data) return 'loading';
  return data.kind === 'unchanged' ? 'found' : data.kind;
});
const errorMessage = computed(() => (query.error.value ? errorText(query.error.value) : ''));

const mounted = ref(false);
let editor: import('monaco-editor').editor.IStandaloneCodeEditor | null = null;

// Re-reads `editor` past an await, where control-flow narrowing would still call it null.
const liveEditor = () => editor;

async function mountEditor(text: string): Promise<void> {
  viewer.setText(text);
  if (editor) {
    editor.getModel()?.setValue(text);
    return;
  }
  const mod = await loadMonaco();
  // Unmounted while the import was in flight.
  if (!container.value || !mounted.value) return;
  // A newer text landed during the import and the earlier call created the editor: apply it.
  const raced = liveEditor();
  if (raced) {
    raced.getModel()?.setValue(text);
    return;
  }
  const uri = taskFileUri(mod, props.branchId, props.path);
  const created = viewer.createEditor(mod, uri, text);
  if (!created) return;
  editor = created;
  const restored = registerEditor(tabId, uri, created);
  applyMountReveal(created, consumeReveal(tabId), restored, null);
}

onMounted(() => {
  mounted.value = true;
});

watch(
  () => query.data.value,
  (data) => {
    if (data?.kind === 'found') void mountEditor(data.text);
  },
  { immediate: true },
);

onUnmounted(() => {
  mounted.value = false;
  viewer.dispose();
  editor = null;
  // The viewer is keyed by path: the file is gone from view, so its model goes with it.
  dropRepoFileTab(tabId);
});
</script>

<template>
  <div class="h-full min-h-0 w-full" data-testid="ade-files-viewer" :data-path="path">
    <template v-if="state === 'loading' || state === 'found'">
      <div v-if="isMarkdown" class="flex flex-col h-full w-full">
        <ViewToolbar>
          <SecondaryTabs
            variant="segmented"
            :model-value="view"
            :items="viewOptions"
            data-testid="ade-files-view-toggle"
            @update:model-value="(v) => onViewChange(v as 'source' | 'reading')"
          />
        </ViewToolbar>
        <!-- v-show, never v-if: the editor survives the toggle with its scroll and selection. -->
        <div
          v-show="view === 'source'"
          ref="container"
          class="monaco-host flex-1 min-h-0"
          data-testid="ade-files-editor"
        />
        <!-- markdown-it's html: false escapes literal HTML in the source, so this v-html is safe. -->
        <div
          v-if="view === 'reading'"
          ref="readingPane"
          class="kira-prose flex-1 min-h-0 overflow-auto text-fg bg-bg font-ui text-kira-md leading-normal p-3 [&>*]:max-w-prose [&>pre]:max-w-none [&>table]:max-w-none"
          data-testid="ade-files-markdown"
          v-html="renderedHtml"
        />
      </div>
      <div
        v-else
        ref="container"
        class="monaco-host h-full w-full"
        data-testid="ade-files-editor"
      />
    </template>
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
      :title="t('space.repo.view.openFileFailed.error')"
      :description="errorMessage || undefined"
      data-testid="ade-files-viewer-error"
    />
  </div>
</template>
