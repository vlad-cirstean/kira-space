<script setup lang="ts">
/**
 * The toolbar's Pull control — a split button, `[Pull] [▾]`, mirroring
 * `RefsPopover.vue`'s own trigger/panel shape but at toolbar scale. The main button runs Pull
 * following the resolved default (the ladder, no override); the chevron opens a small popover
 * offering the three explicit strategies plus that same default, each running Pull immediately
 * on selection rather than staging a separate confirm step — a second click to *run* what the
 * user just picked would be friction never asked for (only "show it before it runs", which
 * `OpsState.runPull` already guarantees by resolving-then-announcing before `remote.run`).
 *
 * The popover's own "Default" row previews the ladder's live resolution (`previewPullStrategy`)
 * the moment it opens — a read, not a commitment — so a user who has never run Pull this session
 * still sees "rebase, from your pull.rebase setting" before choosing anything.
 */

import type { PullStrategy, PullStrategySource } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SplitButton from '@theme/components/SplitButton.vue';
import { DropdownMenuContent } from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import MenuItems from '@workbench/components/MenuItems.vue';
import { computed, ref, watch } from 'vue';
import { type MenuSection, sectionsToMenuItems } from '../lib/menuModel.ts';
import { describePullStrategySource, pullStrategyLabel } from '../lib/pullStrategyModel.ts';
import type { OpsState } from '../state/ops.ts';

const props = defineProps<{
  ops: OpsState;
  remote: string;
  branch: string;
  disabled: boolean;
}>();

const isOpen = ref(false);
type Preview =
  | { kind: 'resolving' }
  | { kind: 'ready'; strategy: PullStrategy; source: PullStrategySource };
const preview = ref<Preview | undefined>(undefined);

const lastRun = computed(() => props.ops.pullStrategy.value);
const mainTitle = computed(() => {
  const last = lastRun.value;
  return last
    ? t('git.toolbar.pullLast.tooltip', {
        strategy: pullStrategyLabel(last.strategy),
        source: describePullStrategySource(last.source),
      })
    : t('git.toolbar.pull.hint');
});

const STRATEGIES: readonly PullStrategy[] = ['ff-only', 'merge', 'rebase'];
const DEFAULT_ID = 'pull-strategy-default';

// One section, "Follow your configuration" (the live-resolved preview as its `detail` line) plus
// the three explicit strategies, rendered by `MenuItems` (testids `menu-item-pull-strategy-*`).
const menuSections = computed<MenuSection[]>(() => [
  {
    items: [
      {
        id: DEFAULT_ID,
        label: t('git.toolbar.pullDefault.label'),
        disabled: false,
        disabledReason: undefined,
        detail:
          preview.value?.kind === 'resolving'
            ? t('git.toolbar.pullResolving.loading')
            : preview.value
              ? t('common.withDetail.label', {
                  first: pullStrategyLabel(preview.value.strategy),
                  second: describePullStrategySource(preview.value.source),
                })
              : undefined,
      },
      ...STRATEGIES.map((strategy) => ({
        id: `pull-strategy-${strategy}`,
        label: pullStrategyLabel(strategy),
        disabled: false,
        disabledReason: undefined,
      })),
    ],
  },
]);

function onMenuSelect(id: string): Promise<void> {
  if (id === DEFAULT_ID) return runDefault();
  return runWith(id.slice('pull-strategy-'.length) as PullStrategy);
}

const menuItems = computed(() => sectionsToMenuItems(menuSections.value, onMenuSelect));

function close(): void {
  isOpen.value = false;
}

// Drops a slow preflight answer that no longer matches the current open/branch.
let previewSeq = 0;

async function onOpenChange(value: boolean): Promise<void> {
  isOpen.value = value;
  if (!value) return;
  const seq = ++previewSeq;
  preview.value = { kind: 'resolving' };
  let next: Preview | undefined;
  try {
    const pre = await props.ops.previewPullStrategy(props.branch);
    if (pre) next = { kind: 'ready', strategy: pre.strategy, source: pre.source };
  } catch {
    // The preview is a hint only; without it the item shows no detail and Pull still works.
  }
  if (seq === previewSeq) preview.value = next;
}

watch(
  () => props.branch,
  () => {
    previewSeq++;
    preview.value = undefined;
  },
);

async function runDefault(): Promise<void> {
  close();
  await props.ops.runPull(props.remote, props.branch);
}

async function runWith(strategy: PullStrategy): Promise<void> {
  close();
  await props.ops.runPull(props.remote, props.branch, strategy);
}

// G10 D17: forwarded so App.vue's palette dispatcher's `pull` action reaches the same default-
// strategy path the toolbar's own Pull button click does.
defineExpose({ run: runDefault });
</script>

<template>
  <SplitButton
    :open="isOpen"
    :disabled="disabled"
    :tooltip="mainTitle"
    :chevron-label="t('git.toolbar.pullOptions.aria')"
    chevron-testid="pull-strategy-trigger"
    data-testid="pull-button"
    @click="runDefault"
    @update:open="onOpenChange"
  >
    <CodiconIcon name="repo-pull" :size="13" />
    {{ t('common.action.pull.label') }}
    <template #menu>
      <DropdownMenuContent align="start" :aria-label="t('git.toolbar.pullStrategy.aria')">
        <MenuItems :items="menuItems" />
      </DropdownMenuContent>
    </template>
  </SplitButton>
</template>
