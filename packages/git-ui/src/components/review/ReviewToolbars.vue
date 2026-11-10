<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { type MessageKey, t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import BranchPicker from '@workbench/components/pickers/BranchPicker.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import type { PickerOption } from '@workbench/lib/pickerOptions';
import { computed, nextTick, useTemplateRef, watch } from 'vue';
import { ACTION_ICONS } from '../../icons/index.ts';
import {
  baseCandidateOptions,
  branchGroups,
  branchOptions,
  reasonLabel,
  SUGGESTED_PREFIX,
} from '../../lib/refOptions.ts';
import { useFieldFocus } from '../../lib/useFieldFocus.ts';
import type { FileListMode } from '../../state/detail.ts';
import type { RefsState } from '../../state/refs.ts';
import type { ReviewPane, ReviewSessionState } from '../../state/review.ts';

// The review view's two toolbars: the compared sides (branch, base, swap) and, once listing, the
// panel-level pane toggle, file filter and Tree/Flat toggle that every pane shares.
const props = defineProps<{
  review: ReviewSessionState;
  refsState: RefsState;
  filesCount: number;
  commentsCount: number;
  /** The compared sides are fixed: no back, swap or base pick. */
  locked?: boolean;
  /** No commits, only uncommitted files: no Commits pane, the toolbar still shows. */
  workingOnly?: boolean;
}>();
const emit = defineEmits<{ back: [] }>();
const listMode = defineModel<FileListMode>('listMode', { required: true });
const filter = defineModel<string>('filter', { required: true });
const filterVisible = defineModel<boolean>('filterVisible', { required: true });

// Focus is explicit: `autofocus` is a11y-linted outside modals and fires only on first render.
const toolbarFilter = useTemplateRef<{ $el: Element }>('toolbarFilter');
const focusToolbarFilter = useFieldFocus(toolbarFilter);
watch(filterVisible, (visible) => {
  if (visible) void nextTick(focusToolbarFilter);
});

// Suggested bases first, then every local and remote branch.
const baseOptions = computed<PickerOption[]>(() => [
  ...baseCandidateOptions(props.review.resolution.value),
  ...branchOptions(props.refsState, { remote: true }),
]);
const baseReason = computed(() => {
  const resolution = props.review.resolution.value;
  return resolution && resolution.base !== null ? reasonLabel(resolution.reason) : undefined;
});
// `head` stays undefined until the first refs load lands, so an empty list then is not "no match".
const refsLoading = computed(() => props.refsState.head.value === undefined);

function pickBase(option: PickerOption): void {
  props.review.setBase(
    option.value.startsWith(SUGGESTED_PREFIX)
      ? option.value.slice(SUGGESTED_PREFIX.length)
      : option.value,
  );
}

const commitsCount = computed(() => {
  const resolution = props.review.resolution.value;
  return resolution?.range.kind === 'ready' ? resolution.range.commitCount : 0;
});

interface ToggleOption {
  readonly id: string;
  readonly icon: string;
  readonly label: MessageKey;
  readonly badge?: number;
}

const panelItems = computed(() => {
  const options: readonly ToggleOption[] = [
    {
      id: 'commits',
      icon: ACTION_ICONS.commits,
      label: 'common.module.commits.label',
      badge: commitsCount.value,
    },
    {
      id: 'files',
      icon: ACTION_ICONS.files,
      label: 'common.module.files.label',
      badge: props.filesCount,
    },
    {
      id: 'comments',
      icon: ACTION_ICONS.comments,
      label: 'git.review.pane.comments.label',
      badge: props.commentsCount,
    },
  ];
  return options
    .filter((o) => !(props.workingOnly && o.id === 'commits'))
    .map((o) => ({
      value: o.id,
      label: t(o.label),
      icon: o.icon,
      count: o.badge,
      tooltip: t(o.label),
      testid: `review-pane-${o.id}`,
    }));
});
const activePane = computed(() =>
  props.workingOnly && props.review.pane.value === 'commits' ? 'files' : props.review.pane.value,
);

const listModeItems = computed(() => {
  const options: readonly ToggleOption[] = [
    { id: 'tree', icon: ACTION_ICONS.listTree, label: 'git.fileTree.listTree.tooltip' },
    { id: 'flat', icon: ACTION_ICONS.listFlat, label: 'git.fileTree.listFlat.tooltip' },
  ];
  return options.map((o) => ({
    value: o.id,
    label: t(o.label),
    icon: o.icon,
    tooltip: t(o.label),
  }));
});

/** The comparison's second line, "N commits · M files changed". */
const summaryLabel = computed(() => {
  const resolution = props.review.resolution.value;
  if (resolution?.range.kind !== 'ready') return '';
  return t('common.dotPair.label', {
    first: t('git.review.commits.count.label', { count: resolution.range.commitCount }),
    second: t('git.review.filesChanged.label', { count: props.filesCount }),
  });
});
</script>

<template>
  <!-- The back button renders whenever a branch is picked, the error phase included: a resumed
       session pointing at a deleted branch or base lands there. -->
  <ViewToolbar>
    <TooltipIconButton
      v-if="!locked"
      icon="chevron-left"
      :label="t('git.review.back.tooltip')"
      data-testid="review-back-button"
      @click="emit('back')"
    />
    <CodiconIcon name="git-branch" :size="13" class="shrink-0 text-muted-foreground" />
    <span
      class="min-w-0 truncate font-data text-kira-md text-fg"
      data-testid="review-branch-name"
      >{{
        review.branch.value
      }}</span
    >
    <span class="text-muted-foreground shrink-0" aria-hidden="true">↔</span>
    <span
      v-if="locked"
      class="min-w-0 truncate font-data text-kira-md text-fg"
      data-testid="review-base-name"
      >{{
        review.resolution.value?.base
      }}</span
    >
    <BranchPicker
      v-else
      size="kira"
      testid="base-selector"
      :model-value="review.resolution.value?.base ?? null"
      :branches="baseOptions"
      :groups="branchGroups()"
      :placeholder="t('git.review.base.choose.label')"
      :trigger-detail="baseReason"
      :loading="refsLoading"
      @select="pickBase"
    />
    <TooltipIconButton
      v-if="!locked"
      icon="arrow-swap"
      :label="t('git.review.swap.tooltip')"
      data-testid="review-swap-button"
      @click="review.swapBaseAndBranch()"
    />
    <span
      v-if="review.phase.value === 'listing'"
      class="ml-auto min-w-0 truncate text-kira-sm text-subtle"
    >
      {{ summaryLabel }}
    </span>
  </ViewToolbar>

  <ViewToolbar v-if="review.phase.value === 'listing' || workingOnly" data-testid="review-toolbar">
    <SecondaryTabs
      :aria-label="t('git.review.pane.aria')"
      :model-value="activePane"
      :items="panelItems"
      @update:model-value="(pane) => review.setPane(pane as ReviewPane)"
    >
      <template #item="{ item }">
        <CodiconIcon :name="item.icon ?? ''" :size="13" />
        <span class="sr-only">{{ item.label }}</span>
        <span v-if="item.count !== undefined" :class="rowMeta">{{ item.count }}</span>
      </template>
    </SecondaryTabs>
    <!-- Pressed while the field is revealed or a filter is applied but collapsed. -->
    <TooltipIconButton
      icon="search"
      :label="t('git.fileTree.filter.placeholder')"
      :pressed="filterVisible || filter.length > 0"
      data-testid="review-filter-toggle"
      @click="filterVisible = !filterVisible"
    />
    <div v-if="filterVisible" class="min-w-0 flex-1">
      <SearchField
        ref="toolbarFilter"
        v-model="filter"
        data-testid="review-file-filter"
        :placeholder="t('git.fileTree.filter.placeholder')"
        :aria-label="t('git.fileTree.filter.placeholder')"
      />
    </div>
    <SecondaryTabs
      variant="segmented"
      :aria-label="t('git.fileTree.display.aria')"
      :model-value="listMode"
      :items="listModeItems"
      @update:model-value="(v) => (listMode = v as FileListMode)"
    >
      <template #item="{ item }">
        <CodiconIcon :name="item.icon ?? ''" :size="13" />
        <span class="sr-only">{{ item.label }}</span>
      </template>
    </SecondaryTabs>
  </ViewToolbar>
</template>
