<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Button } from '@theme/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { type ComponentPublicInstance, nextTick, onUnmounted, type Ref, reactive, ref } from 'vue';
// One commit's whole changed-file set, one tab — VS Code's own multi-file diff
// editor's counterpart. A scrolling column of per-file sections; each section's own diff editor
// mounts only while that section is expanded, so a 60-file commit constructs at most a few live
// Monaco instances rather than sixty at once. First section expanded on mount, the rest collapsed.
//
// Each section reuses RepoDiffView.vue's own loading path verbatim (useDiffEditor.ts), keyed
// `${tabId}:${path}` — editors.ts's own composite-key convention for this tab kind
// (dropRepoMultiDiffTab's own doc comment) — rather than a second copy of that five-state
// classification (CLAUDE.md's "one term per concept").
//
// A section's `useDiffEditor` handle (and the container element it mounts into) deliberately live
// in plain Maps, not inside the `sections` reactive array: a DiffEditorHandle carries its own Refs
// (`state`/`errorMessage`), and `reactive()`'s deep unwrap would strip that Ref-ness from a nested
// property, breaking DiffEditorHandle's own type. Only the two plain per-path fields the template
// reads — `expanded`, and each section's own copied-out state/error strings — need to be
// reactive.
import type { RepoMultiDiffTabRecord } from '../../state/tabDomain';
import { noRepositoryMessage, repoIdOfTab } from '../../state/workspace';
import { useDiffEditor } from './useDiffEditor';

const props = defineProps<{ tab: RepoMultiDiffTabRecord }>();

type SectionState = 'loading' | 'found' | 'binary' | 'tooLarge' | 'bothMissing' | 'error';

interface SectionMeta {
  path: string;
  expanded: boolean;
}

const repoId = repoIdOfTab(props.tab);

// Plan §7.3: first section expanded on mount, the rest collapsed.
const sections = reactive<SectionMeta[]>(
  props.tab.state.files.map((path, index) => ({ path, expanded: index === 0 })),
);

const sectionState = reactive<Record<string, SectionState>>(
  Object.fromEntries(props.tab.state.files.map((path) => [path, 'loading'])),
);
const sectionError = reactive<Record<string, string>>({});

const containers = new Map<string, Ref<HTMLElement | null>>();
function containerRef(path: string): Ref<HTMLElement | null> {
  let r = containers.get(path);
  if (!r) {
    r = ref(null);
    containers.set(path, r);
  }
  return r;
}

const diffEditors = new Map<string, ReturnType<typeof useDiffEditor>>();

function setContainer(path: string, el: Element | ComponentPublicInstance | null): void {
  const target = containerRef(path);
  target.value = el instanceof HTMLElement ? el : null;
  const meta = sections.find((s) => s.path === path);
  if (meta?.expanded && target.value && !diffEditors.has(path)) void mountSection(path);
}

async function mountSection(path: string): Promise<void> {
  if (!repoId || diffEditors.has(path)) return;
  await nextTick();
  const meta = sections.find((s) => s.path === path);
  if (!meta?.expanded || !containerRef(path).value) return; // Collapsed again before the DOM caught up.
  const handle: ReturnType<typeof useDiffEditor> = useDiffEditor(containerRef(path), {
    isCurrent: () => diffEditors.get(path) === handle,
    editorKey: `${props.tab.id}:${path}`,
    repoId,
    path,
    left: props.tab.state.left,
    right: props.tab.state.right,
    review: props.tab.state.review !== null,
  });
  diffEditors.set(path, handle);
  await handle.mount();
  if (diffEditors.get(path) !== handle) return;
  sectionState[path] = handle.state.value;
  sectionError[path] = handle.errorMessage.value;
}

// Collapsing a section disposes its editor widget only — the models it loaded stay cached in
// editors.ts (RepoDiffView.vue's own onUnmounted comment: "the models survive ... until an actual
// close calls dropResources"), so re-expanding the same section is cheap.
function disposeSection(path: string): void {
  diffEditors.get(path)?.dispose();
  diffEditors.delete(path);
  sectionState[path] = 'loading';
}

function toggle(meta: SectionMeta): void {
  meta.expanded = !meta.expanded;
  if (meta.expanded) {
    // Else: setContainer's own ref callback mounts it once Vue renders the now-expanded template.
    if (containerRef(meta.path).value) void mountSection(meta.path);
  } else {
    disposeSection(meta.path);
  }
}

function goToFile(path: string): void {
  diffEditors.get(path)?.goToFile();
}

function basename(path: string): string {
  const idx = path.lastIndexOf('/');
  return idx < 0 ? path : path.slice(idx + 1);
}

onUnmounted(() => {
  for (const meta of sections) disposeSection(meta.path);
});
</script>

<template>
  <div v-if="!repoId" class="h-full overflow-y-auto flex flex-col">
    <EmptyState fill icon="warning" :title="noRepositoryMessage()" />
  </div>
  <div v-else class="h-full overflow-y-auto flex flex-col" data-testid="repo-multi-diff-view">
    <div v-for="section in sections" :key="section.path" class="flex flex-none flex-col">
      <ViewToolbar>
        <TooltipIconButton
          :icon="section.expanded ? 'chevron-down' : 'chevron-right'"
          :label="
            section.expanded ? t('common.action.collapse.label') : t('common.action.expand.label')
          "
          data-testid="repo-multi-diff-toggle"
          @click="toggle(section)"
        />
        <Tooltip>
          <TooltipTrigger as-child>
            <span class="shrink-0 text-kira-md text-fg whitespace-nowrap">{{
              basename(section.path)
            }}</span>
          </TooltipTrigger>
          <TooltipContent>{{ section.path }}</TooltipContent>
        </Tooltip>
        <span class="path-dir flex-auto min-w-0 text-kira-md text-subtle"
          ><TruncatedText :text="section.path" /></span
        >
        <Button
          v-if="section.expanded && sectionState[section.path] === 'found'"
          variant="toolbar"
          size="kira"
          data-testid="repo-multi-diff-go-to-file"
          @click="goToFile(section.path)"
        >
          <CodiconIcon name="go-to-file" :size="13" />
          {{ t('common.goToFile.label') }}
        </Button>
      </ViewToolbar>
      <template v-if="section.expanded">
        <div
          v-if="sectionState[section.path] === 'loading' || sectionState[section.path] === 'found'"
          :ref="(el) => setContainer(section.path, el)"
          class="monaco-host flex-none h-[60vh] w-full"
          data-testid="repo-multi-diff-editor"
        />
        <EmptyState
          v-else-if="sectionState[section.path] === 'binary'"
          icon="file-binary"
          :title="t('space.repo.view.binaryCompare.empty')"
        />
        <EmptyState
          v-else-if="sectionState[section.path] === 'tooLarge'"
          icon="warning"
          :title="t('space.repo.view.tooLargeCompare.empty')"
        />
        <EmptyState
          v-else-if="sectionState[section.path] === 'bothMissing'"
          icon="warning"
          :title="t('space.repo.view.missing.empty')"
        />
        <EmptyState
          v-else
          icon="warning"
          :title="sectionError[section.path] || t('space.repo.view.openDiffFailed.error')"
        />
      </template>
    </div>
    <EmptyState
      v-if="sections.length === 0"
      fill
      icon="git-compare"
      :title="t('space.repo.view.noChanges.empty')"
    />
  </div>
</template>
