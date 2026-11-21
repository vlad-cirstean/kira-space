<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import BranchChip from '@theme/components/BranchChip.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useFocus } from '@vueuse/core';
import { errorText } from '@workbench/bridge/codedError';
import MenuIconButton from '@workbench/components/MenuIconButton.vue';
import { formatShortcut } from '@workbench/shortcuts/keys';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref, useTemplateRef, watch } from 'vue';
import { useRepoLinksStore } from '../../../repo/state/repoLinks';
import { repoTint } from '../../../state/coderepos';
import { useRunAutomation } from '../automation/useRunAutomation';
import { type BranchAction, tagLabel } from '../board/actions';
import { fixItems } from '../board/menu';
import AdeTaskBranchMenu from '../panel/AdeTaskBranchMenu.vue';
import {
  ADE_ROW,
  ADE_ROW_ACTIONS,
  ADE_ROW_BODY,
  ADE_ROW_LEAD,
  rowIndent,
  rowMeta,
} from '../panel/adeRow';
import { useChangeRequests } from '../queries';
import { useReviewCode } from '../review/useReviewCode';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
import { TONE_TEXT_CLASS } from '../tones';
import AdeAttention from './AdeAttention.vue';
import AdeBranchChips from './AdeBranchChips.vue';
import AdeBranchPr from './AdeBranchPr.vue';
import AdeBranchProg from './AdeBranchProg.vue';
import AdeBranchTip from './AdeBranchTip.vue';
import AdeOpStatus from './AdeOpStatus.vue';
import AdeTagIcon from './AdeTagIcon.vue';
import { ATTACH_GLYPH, BRANCH_ACTION_ICON, BRANCH_TAG_GLYPH, rowActionSplit } from './glyphs';
import { BRANCH_GRID, GROUND_ROW, GROUND_SELECTED, HOVER_OVERLAY, SLOT } from './taskGrid';
import { type BranchRowModel, usePlanModel } from './usePlanModel';
import { useSharedRepos } from './useSharedRepos';

const props = withDefaults(
  defineProps<{
    row: BranchRowModel;
    color: string | null;
    taskRow?: boolean;
    testid?: string;
    /** `panel`: one row on the task panel's grid; `board`: the two-line plan row. */
    layout?: 'board' | 'panel';
    /** Repos where the task has a branch slot waiting for a name (the board's edit pencil). */
    draftRepoIds?: readonly string[];
    /** Show the edit pencil (board layout only). */
    editable?: boolean;
  }>(),
  { testid: 'ade-branch-row', layout: 'board', draftRepoIds: () => [], editable: false },
);
const panel = computed(() => props.layout === 'panel');
const emit = defineEmits<{ forcePush: []; taskMenu: [ev: MouseEvent | HTMLElement] }>();
const ui = useAdeBoardUiStore();
const dialogs = useAdeDialogsStore();
const contextMenu = useContextMenuStore();
const tabs = useAdePanelTabsStore();
const takeOver = useAdeTakeOverStore();
const { model, liveNow, rippleIds } = usePlanModel();
const repos = useSharedRepos();
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
const twoLine = computed(() => !panel.value && (props.row.prog !== null || hasChips.value));
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

function failAction(taskId: string, err: unknown): void {
  ui.actionError[taskId] = errorText(err);
  ui.select(taskId);
}

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
    takeOver.request(a.sessionId ?? '').catch((err: unknown) => failAction(row.branch.taskId, err));
  } else if (a.kind === 'start') {
    dialogs.start(row.id).catch((err: unknown) => failAction(row.branch.taskId, err));
  }
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

function onMenuKey(): void {
  const el = rowRef.value;
  if (panel.value || !el) return;
  if (props.taskRow) emit('taskMenu', el);
  else contextMenu.openContextMenuFor(el, menuItems());
}
useShortcut(rowRef, ['menu.open'], onMenuKey);

const baseName = computed(() => {
  const part = props.row.base?.label.find((p) => typeof p !== 'string');
  return part && typeof part !== 'string' ? part.value : '';
});
const tagProps = computed(() => ({
  glyph: BRANCH_TAG_GLYPH[props.row.tag.kind],
  kind: props.row.tag.kind,
  label: tagText.value,
  tip: props.row.tag.tip,
  tipParts: props.row.tag.tipParts,
  tone: props.row.tag.tone,
}));
// The PR glyph already shows a merged change request, so the tag slot stays empty then.
const crs = useChangeRequests();
const prMerged = computed(() => crs.data.value?.branches[props.row.id]?.state === 'merged');
const tagRedundant = computed(() => props.row.tag.kind === 'merged' && prMerged.value);
const baseVariant = computed(() => (props.row.base?.pending ? 'warn' : 'default'));
</script>

<template>
  <!-- biome-ignore lint/a11y/useSemanticElements: the row holds block content a button cannot. -->
  <div
    ref="rowEl"
    class="group group/row"
    :class="[
      panel
        ? ADE_ROW
        : ['box-border flex w-full items-center gap-1.5 px-2', twoLine ? 'h-board-row' : 'h-row'],
      panel ? (selected ? 'bg-select' : '') : selected ? GROUND_SELECTED : GROUND_ROW,
    ]"
    :style="panel ? rowIndent(row.depth) : undefined"
    :data-slot="panel ? 'ade-row' : undefined"
    role="button"
    tabindex="0"
    :data-selected="selected || undefined"
    :data-ripple="rippleIds.branches.has(row.id) || undefined"
    :data-testid="testid"
    :data-branch-id="row.id"
    :data-depth="row.depth"
    @click="pick"
    @contextmenu="onMenu"
    @keydown.enter.self="pick"
    @keydown.space.self.prevent="pick"
  >
    <template v-if="panel">
      <span data-slot="ade-row-lead" :class="ADE_ROW_LEAD">
        <span
          v-if="row.depth"
          class="absolute -left-1 -top-1 box-border h-4.5 w-1.5 rounded-bl-kira-sm border-b-2 border-l-2 border-border"
        />
        <AdeTagIcon v-bind="tagProps" />
      </span>
      <AdeBranchTip v-model:open="tipOpen" :row="row" :color="color">
        <div data-slot="ade-row-body" :class="ADE_ROW_BODY">
          <Badge
            variant="chip"
            :class="repoTintOf.class"
            :style="repoTintOf.style"
            data-testid="ade-branch-repo"
            >{{
              row.repo
            }}</Badge
          >
          <AdeOpStatus :task-id="row.branch.taskId" :branch-id="row.id" />
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
          <div class="flex min-w-0 flex-1 items-center gap-1.5 text-left">
            <span class="flex min-w-0 items-center gap-1">
              <span
                class="truncate font-data text-kira-md font-medium"
                :class="
                  row.draft || row.attach?.state === 'detached'
                    ? 'text-muted-foreground'
                    : 'text-fg'
                "
                data-testid="ade-branch-name"
                >{{
                  row.name
                }}</span
              >
              <TextTooltip v-if="row.attach" :text="row.attach.tip">
                <span
                  role="img"
                  class="inline-flex shrink-0 text-subtle"
                  :aria-label="row.attach.tip"
                  :data-testid="`ade-branch-${row.attach.state}`"
                >
                  <CodiconIcon :name="ATTACH_GLYPH[row.attach.state].icon" :size="13" />
                </span>
              </TextTooltip>
              <TextTooltip v-if="row.checkedOut" :text="row.attach?.tip ?? ''">
                <BranchChip
                  :name="t('space.ade.plan.branchRow.checkedOut.label', { branch: row.checkedOut })"
                  variant="warn"
                  class="max-w-32 shrink-0"
                  data-testid="ade-branch-checked-out"
                />
              </TextTooltip>
            </span>
            <span
              v-if="row.context || row.prog || hasChips"
              class="flex min-w-0 items-center gap-1.5 overflow-hidden whitespace-nowrap"
            >
              <AdeBranchProg v-if="row.prog" :prog="row.prog" />
              <span
                v-if="row.context"
                class="truncate"
                :class="rowMeta"
                data-testid="ade-branch-context"
                >{{
                  row.context
                }}</span
              >
              <AdeBranchChips :chips="row.chips" />
            </span>
          </div>
          <AdeAttention v-if="row.attention" :tip="row.attention" :item="row.attentionItem" />
        </div>
      </AdeBranchTip>
      <div data-slot="ade-row-actions" :class="ADE_ROW_ACTIONS">
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
          :kbd="formatShortcut('ade.reviewCode')"
          class="shrink-0"
          disabled-trigger
          :pending="reviewCode.pendingFor([row.id])"
          :disabled="choice.disabled"
          data-testid="ade-branch-review"
          @click.stop="reviewCode.open(row.branch.taskId, row.id, rowRef)"
        >
          <VarText :parts="choice.tip" />
        </TooltipIconButton>
        <slot />
      </div>
    </template>
    <template v-else>
      <span class="relative shrink-0 self-stretch" :style="{ width: `${row.depth * 16}px` }">
        <span
          v-if="row.depth"
          class="absolute -top-1 right-0.5 box-border h-4.5 w-2 rounded-bl-kira-sm border-b-2 border-l-2 border-border"
        />
      </span>
      <div :class="BRANCH_GRID">
        <Badge
          variant="chip"
          class="max-w-full justify-self-start"
          :class="repoTintOf.class"
          :style="repoTintOf.style"
          data-testid="ade-branch-repo"
          ><span class="truncate">{{ row.repo }}</span></Badge
        >
        <div class="relative flex h-control min-w-0 items-center gap-1.5">
          <AdeBranchTip v-model:open="tipOpen" :row="row" :color="color">
            <span class="flex min-w-0 shrink items-center gap-1.5">
              <Badge v-if="row.isReview" data-testid="ade-owner-pill">{{ row.branch.owner }}</Badge>
              <span
                v-if="row.branch.origin === 'agent'"
                class="inline-flex shrink-0 text-subtle"
                data-testid="ade-branch-agent"
              >
                <CodiconIcon name="sparkle" :size="13" />
              </span>
              <span
                class="truncate font-data text-kira-md font-normal"
                :class="
                  row.draft || row.attach?.state === 'detached'
                    ? 'text-subtle'
                    : 'text-muted-foreground group-hover/row:text-fg group-data-selected/row:text-fg'
                "
                data-testid="ade-branch-name"
                >{{
                  row.name
                }}</span
              >
              <TextTooltip v-if="row.attach" :text="row.attach.tip">
                <span
                  role="img"
                  class="inline-flex shrink-0 text-subtle"
                  :aria-label="row.attach.tip"
                  :data-testid="`ade-branch-${row.attach.state}`"
                >
                  <CodiconIcon :name="ATTACH_GLYPH[row.attach.state].icon" :size="13" />
                </span>
              </TextTooltip>
              <TextTooltip v-if="row.checkedOut" :text="row.attach?.tip ?? ''">
                <BranchChip
                  :name="t('space.ade.plan.branchRow.checkedOut.label', { branch: row.checkedOut })"
                  variant="warn"
                  class="max-w-24 shrink-0"
                  data-testid="ade-branch-checked-out"
                />
              </TextTooltip>
              <TextTooltip v-if="row.base && (baseName || row.base.pending)" :parts="row.base.tip">
                <span
                  class="inline-flex min-w-0 max-w-20 items-center gap-0.5 font-data text-kira-sm"
                  :class="
                    row.base.pending
                      ? TONE_TEXT_CLASS.amber
                      : row.base.tone === 'blue'
                        ? TONE_TEXT_CLASS.blue
                        : 'text-subtle'
                  "
                  data-testid="ade-base-marker"
                  :data-pending="row.base.pending || undefined"
                >
                  <CodiconIcon name="git-branch" :size="12" class="shrink-0" />
                  <span v-if="baseName" class="truncate">{{ baseName }}</span>
                </span>
              </TextTooltip>
            </span>
          </AdeBranchTip>
          <span
            v-if="row.context"
            class="min-w-0 flex-1 basis-0"
            :class="rowMeta"
            data-testid="ade-branch-context"
            ><TruncatedText :text="row.context" /></span
          >
          <span v-else class="flex-1" />
          <span
            class="hidden group-hover/row:flex group-focus-visible/row:flex group-has-[:focus-visible]/row:flex has-[[data-state=open]]:flex has-[[data-pinned]]:flex"
            :class="HOVER_OVERLAY"
            data-testid="ade-branch-actions"
          >
            <AdeTaskBranchMenu
              v-if="editable"
              align="end"
              :row="row"
              :draft-repo-ids="draftRepoIds"
            />
            <TooltipIconButton
              v-for="a in split.shown.filter((x) => VARIANT[x.kind] === 'dialog')"
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
            <TooltipIconButton
              v-if="choice"
              icon="git-compare"
              :label="t('space.ade.board.taskMenu.reviewCode.label')"
              :kbd="formatShortcut('ade.reviewCode')"
              class="size-5 shrink-0"
              disabled-trigger
              :pending="reviewCode.pendingFor([row.id])"
              :disabled="choice.disabled"
              data-testid="ade-branch-review"
              @click.stop="reviewCode.open(row.branch.taskId, row.id, rowRef)"
            >
              <VarText :parts="choice.tip" />
            </TooltipIconButton>
          </span>
        </div>
        <div class="flex shrink-0 items-center gap-1">
          <AdeOpStatus :task-id="row.branch.taskId" :branch-id="row.id" />
          <AdeAttention v-if="row.attention" :tip="row.attention" :item="row.attentionItem" />
          <TooltipIconButton
            v-for="a in split.shown.filter((x) => VARIANT[x.kind] === 'dialog-danger')"
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
          <slot />
        </div>
        <span :class="SLOT"><AdeBranchPr :row="row" /></span>
        <span :class="SLOT"><AdeTagIcon v-if="!tagRedundant" v-bind="tagProps" /></span>
        <AdeBranchTip v-if="twoLine" :row="row" :color="color">
          <div
            class="col-start-2 col-end-[-1] flex h-4.5 min-w-0 items-center gap-1.5 overflow-hidden whitespace-nowrap"
            data-testid="ade-branch-line2"
          >
            <AdeBranchProg v-if="row.prog" :prog="row.prog" />
            <span class="flex-1" />
            <span class="flex shrink-0 items-center gap-1"
              ><AdeBranchChips :chips="row.chips" /></span
            >
          </div>
        </AdeBranchTip>
      </div>
    </template>
  </div>
</template>
