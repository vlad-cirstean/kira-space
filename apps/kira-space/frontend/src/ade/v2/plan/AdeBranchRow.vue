<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import BranchChip from '@theme/components/BranchChip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { Kbd } from '@theme/components/ui/kbd';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useFocus } from '@vueuse/core';
import { errorText } from '@workbench/bridge/codedError';
import MenuIconButton from '@workbench/components/MenuIconButton.vue';
import { formatShortcut } from '@workbench/shortcuts/keys';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref, useTemplateRef, watch } from 'vue';
import { useRepoLinksStore } from '../../../repo/state/repoLinks';
import { useRepos } from '../../../repo/state/reposQueries';
import { repoTint } from '../../../state/coderepos';
import { useRunAutomation } from '../automation/useRunAutomation';
import { type BranchAction, tagLabel } from '../board/actions';
import { fixItems } from '../board/menu';
import { useReviewCode } from '../review/useReviewCode';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
import AdeAttention from './AdeAttention.vue';
import AdeBranchTip from './AdeBranchTip.vue';
import AdeTagIcon from './AdeTagIcon.vue';
import { BRANCH_ACTION_ICON, BRANCH_TAG_GLYPH, rowActionSplit } from './glyphs';
import { type BranchRowModel, usePlanModel } from './usePlanModel';

const props = withDefaults(
  defineProps<{ row: BranchRowModel; color: string | null; taskRow?: boolean; testid?: string }>(),
  { testid: 'ade-branch-row' },
);
const emit = defineEmits<{ forcePush: []; taskMenu: [ev: MouseEvent] }>();
const ui = useAdeBoardUiStore();
const dialogs = useAdeDialogsStore();
const contextMenu = useContextMenuStore();
const tabs = useAdePanelTabsStore();
const takeOver = useAdeTakeOverStore();
const { model, liveNow } = usePlanModel();
const repos = useRepos();
const repoLinks = useRepoLinksStore();
const repoTintOf = computed(() => repoTint(repoLinks.repoColorOf(props.row.branch.codeRepoId)));
const reviewCode = useReviewCode();
const automation = useRunAutomation();
const choice = computed(() => reviewCode.choiceOf(props.row));
const rowRef = useTemplateRef<HTMLElement>('rowEl');
const tipOpen = ref(false);
const { focused } = useFocus(rowRef);
watch(focused, (f) => {
  tipOpen.value = f && rowRef.value?.matches(':focus-visible') === true;
});
const hasChips = computed(
  () => props.row.chips.merged.length + props.row.chips.deployed.length > 0,
);
const selected = computed(() =>
  props.taskRow
    ? ui.selectedTaskId === props.row.branch.taskId && ui.selectedBranchId === null
    : ui.selectedBranchId === props.row.id,
);

// A review card has no head, so its row stands for the task.
function pick(): void {
  if (props.taskRow) ui.select(props.row.branch.taskId);
  else ui.selectBranch(props.row.branch.taskId, props.row.id);
}

const VARIANT: Record<BranchAction['kind'], 'dialog' | 'dialog-danger'> = {
  forcePush: 'dialog',
  rebase: 'dialog',
  queue: 'dialog-danger',
  changeBase: 'dialog',
  abortRebase: 'dialog-danger',
  seeLog: 'dialog',
  takeOver: 'dialog-danger',
  seeError: 'dialog-danger',
  start: 'dialog',
};
const TESTID: Record<BranchAction['kind'], string> = {
  forcePush: 'ade-force-push',
  rebase: 'ade-rebase',
  queue: 'ade-queue-after',
  changeBase: 'ade-change-base',
  abortRebase: 'ade-abort-rebase',
  seeLog: 'ade-see-log',
  takeOver: 'ade-take-over',
  seeError: 'ade-see-error',
  start: 'ade-start',
};
// Rebase acts and force push are already in the fix items; the overflow adds the rest.
const MENU_KINDS: readonly BranchAction['kind'][] = ['seeError', 'seeLog', 'takeOver', 'start'];

const tagText = computed(() => tagLabel(props.row.tag, liveNow.value.getTime()));
const split = computed(() => rowActionSplit(props.row.tag.actions));

function act(a: BranchAction): void {
  const row = props.row;
  if (a.act) dialogs.act(a.act);
  else if (a.kind === 'forcePush') emit('forcePush');
  else if (a.kind === 'seeError') {
    ui.selectBranch(row.branch.taskId, row.id);
    tabs.setBranchTab(row.id, 'details');
    ui.focusSetup = true;
  } else if (a.kind === 'seeLog') {
    ui.selectBranch(row.branch.taskId, row.id);
    tabs.setBranchTab(row.id, 'details');
    ui.focusRebaseLog = true;
  } else if (a.kind === 'takeOver') {
    delete ui.actionError[row.branch.taskId];
    takeOver.request(a.sessionId ?? '').catch((err: unknown) => {
      ui.actionError[row.branch.taskId] = errorText(err);
      ui.select(row.branch.taskId);
    });
  } else if (a.kind === 'start') dialogs.start(row.id);
}

function menuItems(): MenuItem[] {
  const m = model.value;
  const items = m
    ? fixItems({
        branchId: props.row.id,
        graph: m.view.graph,
        targets:
          repos.data.value?.repos.find((r) => r.codeRepoId === props.row.branch.codeRepoId)
            ?.integrationBranches ?? [],
        after: m.view.after,
        unpushed: m.board.plan.unpushed,
        runs: m.view.graph.byTask.get(props.row.branch.taskId)?.runs ?? [],
        mainName:
          m.board.repos.find((r) => r.codeRepoId === props.row.branch.codeRepoId)?.mainName ?? '',
      })
    : null;
  const id = props.row.id;
  return [
    {
      type: 'label',
      label: t('space.ade.plan.branchRow.menuTitle.label', {
        name: props.row.name,
        repo: props.row.repo,
      }),
    },
    ...(choice.value
      ? [
          {
            type: 'item' as const,
            id: 'ade-review-code',
            label: t('space.ade.board.taskMenu.reviewCode.label'),
            disabled: choice.value.disabled,
            hint: choice.value.tip,
            shortcut: 'ade.reviewCode' as const,
            run: () => reviewCode.open(props.row.branch.taskId, id, rowRef.value),
          },
        ]
      : []),
    ...(props.row.branch.kind === 'mine' && props.row.branch.name !== ''
      ? [automation.submenu(props.row.branch.taskId, id)]
      : []),
    ...(!items
      ? []
      : items.length === 0
        ? [
            {
              type: 'item' as const,
              id: 'ade-fix-none',
              label: t('space.ade.board.fixMenu.none.label'),
              disabled: true,
              run: () => {},
            },
          ]
        : items.map((it) => ({
            type: 'item' as const,
            id: it.id,
            label: it.label,
            disabled: it.kind === 'act' ? it.act.disabled : undefined,
            hint: it.kind === 'act' ? it.hint : undefined,
            run: () => {
              if (it.kind === 'merge') dialogs.merge(id, it.target);
              else if (it.kind === 'act') dialogs.act(it.act);
              else emit('forcePush');
            },
          }))),
    ...split.value.overflow
      .filter((a) => MENU_KINDS.includes(a.kind))
      .map((a) => ({
        type: 'item' as const,
        id: `ade-row-action-${a.kind}`,
        label: a.label,
        disabled: a.disabled,
        hint: a.tipParts,
        run: () => act(a),
      })),
  ];
}

function onMenu(ev: MouseEvent): void {
  if (props.taskRow) {
    emit('taskMenu', ev);
    return;
  }
  ev.preventDefault();
  ev.stopPropagation();
  contextMenu.openContextMenu(ev, menuItems());
}

const SEG: Record<string, string> = {
  done: 'bg-muted-foreground',
  running: 'bg-fg',
  bad: 'bg-tone-red-solid',
};
const segClass = (state: string): string => SEG[state] ?? 'bg-border-strong';
const progClass = computed(() => {
  return props.row.prog?.tone === 'red' ? 'text-tone-red' : 'text-muted-foreground';
});
const baseName = computed(() => {
  const part = props.row.base?.label.find((p) => typeof p !== 'string');
  return part && typeof part !== 'string' ? part.value : '';
});
const baseVariant = computed(() => (props.row.base?.pending ? 'warn' : 'default'));
</script>

<template>
  <!-- biome-ignore lint/a11y/useSemanticElements: the row holds block content a button cannot. -->
  <div
    ref="rowEl"
    class="group box-border flex h-board-row w-full items-center gap-1.5 px-2"
    :class="selected ? 'bg-select' : ''"
    role="button"
    tabindex="0"
    :data-selected="selected || undefined"
    :data-ripple="row.ripple || undefined"
    :data-testid="testid"
    :data-branch-id="row.id"
    :data-depth="row.depth"
    @click="pick"
    @contextmenu="onMenu"
    @keydown.enter.self="pick"
    @keydown.space.self.prevent="pick"
  >
    <span class="relative shrink-0 self-stretch" :style="{ width: `${row.depth * 16}px` }">
      <span
        v-if="row.depth"
        class="absolute -top-1 right-0.5 box-border h-4.5 w-2 rounded-bl-kira-sm border-b-2 border-l-2 border-border-strong"
      />
    </span>
    <AdeBranchTip v-model:open="tipOpen" :row="row" :color="color">
      <div class="flex h-full min-w-0 flex-1 items-center gap-1.5">
        <Badge
          variant="chip"
          :class="repoTintOf.class"
          :style="repoTintOf.style"
          data-testid="ade-branch-repo"
          >{{
            row.repo
          }}</Badge
        >
        <BranchChip
          v-if="row.base"
          :name="baseName"
          :variant="baseVariant"
          class="max-w-28"
          data-testid="ade-base-marker"
          :data-pending="row.base.pending || undefined"
        />
        <Badge v-if="row.isReview" data-testid="ade-owner-pill">{{ row.branch.owner }}</Badge>
        <span
          v-if="row.branch.origin === 'agent'"
          class="inline-flex shrink-0 text-subtle"
          data-testid="ade-branch-agent"
        >
          <CodiconIcon name="sparkle" :size="13" />
        </span>
        <div class="flex h-full min-w-0 flex-1 flex-col justify-center text-left">
          <span
            class="truncate font-data text-kira-md font-medium leading-4"
            :class="row.draft ? 'text-muted-foreground' : 'text-fg'"
            data-testid="ade-branch-name"
            >{{
              row.name
            }}</span
          >
          <span
            v-if="row.context || row.prog || hasChips"
            class="flex min-w-0 items-center gap-1.5 overflow-hidden whitespace-nowrap leading-3.5"
          >
            <span
              v-if="row.prog"
              class="inline-flex shrink-0 items-center gap-1"
              data-testid="ade-branch-prog"
            >
              <span class="inline-flex gap-0.5">
                <span
                  v-for="(seg, i) in row.prog.segs"
                  :key="i"
                  class="h-1.25 w-1.75 rounded-kira-xs"
                  :class="segClass(seg)"
                />
              </span>
              <span class="text-kira-sm" :class="progClass">{{ row.prog.label }}</span>
            </span>
            <span
              v-if="row.context"
              class="truncate text-kira-sm text-subtle"
              data-testid="ade-branch-context"
              >{{
                row.context
              }}</span
            >
            <span
              v-for="c in row.chips.merged"
              :key="c.label"
              class="shrink-0 text-kira-sm"
              :class="c.tone === 'stale' ? 'text-tone-amber' : 'text-subtle'"
              data-testid="ade-branch-merged"
              >{{
                c.label
              }}</span
            >
            <span v-if="row.chips.divider" class="h-2.5 w-px shrink-0 bg-border-strong" />
            <span
              v-for="c in row.chips.deployed"
              :key="c.label"
              class="shrink-0 text-kira-sm"
              :class="c.tone === 'stale' ? 'text-tone-amber' : 'text-subtle'"
              data-testid="ade-branch-deployed"
              >{{
                c.label
              }}</span
            >
          </span>
        </div>
      </div>
    </AdeBranchTip>
    <AdeAttention v-if="row.attention" :tip="row.attention" :item="row.attentionItem" />
    <AdeTagIcon
      :glyph="BRANCH_TAG_GLYPH[row.tag.kind]"
      :kind="row.tag.kind"
      :label="tagText"
      :tip="row.tag.tip"
      :tip-parts="row.tag.tipParts"
      :tone="row.tag.tone"
    />
    <TooltipIconButton
      v-for="a in split.shown"
      :key="a.act?.id ?? a.kind"
      :icon="BRANCH_ACTION_ICON[a.kind]"
      :label="a.label"
      :variant="VARIANT[a.kind]"
      size="kira-icon"
      class="shrink-0"
      disabled-trigger
      :disabled="a.disabled"
      :data-testid="TESTID[a.kind]"
      @click.stop="act(a)"
    >
      <div class="font-medium">{{ a.label }}</div>
      <div><VarText :parts="a.tipParts ?? [a.tip]" /></div>
    </TooltipIconButton>
    <MenuIconButton
      v-if="split.overflow.length"
      :icon="ICON.moreActions"
      :label="t('common.moreActions.tooltip')"
      :items="menuItems"
      class="shrink-0"
      data-testid="ade-branch-more"
      @click.stop
    />
    <TooltipIconButton
      v-if="choice"
      icon="git-compare"
      :label="t('space.ade.board.taskMenu.reviewCode.label')"
      class="size-5 shrink-0"
      disabled-trigger
      :disabled="reviewCode.pending.value || choice.disabled"
      data-testid="ade-branch-review"
      @click.stop="reviewCode.open(row.branch.taskId, row.id, rowRef)"
    >
      <VarText :parts="choice.tip" />
      <Kbd>{{ formatShortcut('ade.reviewCode') }}</Kbd>
    </TooltipIconButton>
    <slot />
  </div>
</template>
