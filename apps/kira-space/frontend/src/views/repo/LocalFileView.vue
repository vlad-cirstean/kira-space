<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query';
import EmptyState from '@theme/components/EmptyState.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { control } from '../../bridge/control';
import type { LocalFileTabRecord } from '../../state/tabDomain';
import { useTabsStore } from '../../state/tabs';
import { registerEditor, unmountEditor } from './editors';
import { loadMonaco, localFileUri } from './monaco';
import { applyMountReveal, consumeReveal } from './reveal';
import { useReadonlyFileViewer } from './useReadonlyFileViewer';

// A file opened from outside the app (Finder, a terminal link) that lies in no imported repository:
// read-only Monaco over the file on disk, through OpenedFilesService.Read. No blame. A pending
// reveal places the cursor on mount; a live one is applied by requestReveal.
const props = defineProps<{ tab: LocalFileTabRecord }>();
const tabsStore = useTabsStore();

const viewer = useReadonlyFileViewer({
  tabId: props.tab.id,
  path: props.tab.path,
  markdownReading: props.tab.state.markdownReading,
  persistReading: (reading) =>
    tabsStore.patchLocalFileTabState(props.tab.id, { markdownReading: reading }),
  testidPrefix: 'local-file',
});
// biome-ignore lint/correctness/noUnusedVariables: bound by the template ref="readingPane"
const { container, readingPane, isMarkdown, view, viewOptions, renderedHtml, onViewChange } =
  viewer;

// A remount re-reads: the file may have changed on disk since the last mount, and the editor is
// rebuilt each time anyway, so the cache keeps nothing past unmount.
const query = useQuery({
  queryKey: ['openedFile', props.tab.path],
  queryFn: () => control.openedFilesRead(props.tab.path),
  gcTime: 0,
});

type ViewState = 'loading' | 'found' | 'binary' | 'tooLarge' | 'missing' | 'error';
const state = computed<ViewState>(() => {
  if (query.isError.value) return 'error';
  const data = query.data.value;
  return data ? data.kind : 'loading';
});
const errorMessage = computed(() => (query.error.value ? errorText(query.error.value) : ''));

const mounted = ref(false);
let disposeCursorSub: (() => void) | null = null;

async function mountEditor(text: string): Promise<void> {
  viewer.setText(text);
  const mod = await loadMonaco();
  // Unmounted (tab closed/switched away) while the import was in flight.
  if (!container.value || !mounted.value) return;
  const uri = localFileUri(mod, props.tab.path);
  const editor = viewer.createEditor(mod, uri, text);
  if (!editor) return;
  const restored = registerEditor(props.tab.id, uri, editor);
  applyMountReveal(editor, consumeReveal(props.tab.id), restored, props.tab.state.revealLine);
  const sub = editor.onDidChangeCursorPosition((e) => {
    tabsStore.patchLocalFileTabState(props.tab.id, { revealLine: e.position.lineNumber });
  });
  disposeCursorSub = () => sub.dispose();
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
  disposeCursorSub?.();
  disposeCursorSub = null;
  viewer.dispose();
  unmountEditor(props.tab.id);
});
</script>

<template>
  <template v-if="state === 'loading' || state === 'found'">
    <div v-if="isMarkdown" class="flex flex-col h-full w-full">
      <ViewToolbar border="none">
        <SecondaryTabs
          :model-value="view"
          :items="viewOptions"
          data-testid="local-file-view-toggle"
          @update:model-value="(v) => onViewChange(v as 'source' | 'reading')"
        />
      </ViewToolbar>
      <!-- v-show, never v-if: the editor widget survives the toggle, so scroll position,
           selection and find state survive a round trip. -->
      <div
        v-show="view === 'source'"
        ref="container"
        class="monaco-host flex-1 min-h-0"
        data-testid="local-file-editor"
      />
      <!-- markdown-it's html: false escapes literal HTML in the source, so this v-html is safe. -->
      <div
        v-if="view === 'reading'"
        ref="readingPane"
        class="kira-prose flex-1 min-h-0 overflow-auto text-fg bg-bg font-ui text-kira-md leading-relaxed p-4 [&>*]:max-w-prose [&>pre]:max-w-none [&>table]:max-w-none"
        data-testid="local-file-markdown"
        v-html="renderedHtml"
      />
    </div>
    <div v-else ref="container" class="monaco-host h-full w-full" data-testid="local-file-editor" />
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
    :title="errorMessage || t('space.repo.view.openFileFailed.error')"
  />
</template>
