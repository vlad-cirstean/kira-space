<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { paneFrame } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import { useElementSize, useScroll } from '@vueuse/core';
import { errorText } from '@workbench/bridge/codedError';
import ConfirmDialogView from '@workbench/components/ConfirmDialogView.vue';
import QueryState from '@workbench/components/QueryState.vue';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, nextTick, ref } from 'vue';
import { useSettingsStore } from '../../../state/settings';
import AdeForcePushDialog from '../AdeForcePushDialog.vue';
import { dayLabel, firstWork, isoToOffset, LATER, nextWork } from '../board/calendar';
import { dropVerdict, shiftPlanArgs } from '../board/dropPlan';
import { dayMenuFor } from '../board/menu';
import { refreshNote, remoteErrorText } from '../board/panelFacts';
import { ITEM_LIMIT, type TimelineBand, WINDOW_DAYS } from '../board/timeline';
import { useRefresh, useSetPlan } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useConfirmRun } from '../useConfirmRun';
import AdeDayBand from './AdeDayBand.vue';
import AdeHistoryBar from './AdeHistoryBar.vue';
import AdeHistoryControl from './AdeHistoryControl.vue';
import AdePlanRepos, { type RepoChipModel } from './AdePlanRepos.vue';
import { usePlanDrag } from './usePlanDrag';
import type { BranchRowModel } from './usePlanModel';
import { usePlanModel } from './usePlanModel';

const ui = useAdeBoardUiStore();
const settingsStore = useSettingsStore();
const contextMenu = useContextMenuStore();
const { model, today, repoLabel, settings, boardQuery } = usePlanModel();
const setPlan = useSetPlan();
const refresh = useRefresh();

const scrollEl = ref<HTMLElement | null>(null);
const headerEl = ref<HTMLElement | null>(null);
const { height: headerHeight } = useElementSize(headerEl);
const { y: scrollY } = useScroll(scrollEl);

// ---- refresh
const refreshingIds = ref<string[]>([]);
const refreshingAll = ref(false);
const refreshErrors = ref<Record<string, string>>({});

async function runRefresh(codeRepoIds: string[], all: boolean): Promise<void> {
  const ids = all ? (model.value?.board.repos.map((r) => r.codeRepoId) ?? []) : codeRepoIds;
  refreshingIds.value = [...refreshingIds.value, ...ids];
  refreshingAll.value = refreshingAll.value || all;
  const errs = { ...refreshErrors.value };
  for (const id of ids) delete errs[id];
  try {
    const res = await refresh.mutateAsync({ codeRepoIds: ids });
    const summaries = { ...ui.refreshSummary };
    for (const r of res.repos) {
      if (r.error) errs[r.codeRepoId] = remoteErrorText(r.error);
      else summaries[r.codeRepoId] = refreshNote(r);
    }
    ui.refreshSummary = summaries;
  } catch (err) {
    const msg = errorText(err);
    for (const id of ids) errs[id] = msg;
  } finally {
    refreshErrors.value = errs;
    refreshingIds.value = refreshingIds.value.filter((id) => !ids.includes(id));
    if (all) refreshingAll.value = false;
  }
}

const chips = computed<RepoChipModel[]>(() =>
  (model.value?.board.repos ?? []).map((r) => ({
    codeRepoId: r.codeRepoId,
    label: repoLabel(r.codeRepoId),
    remote: r.remote,
    lastFetchAt: r.lastFetchAt,
    shown: !ui.hiddenRepoIds.includes(r.codeRepoId),
    pending: refreshingIds.value.includes(r.codeRepoId),
    error: refreshErrors.value[r.codeRepoId] ?? '',
    summary: ui.refreshSummary[r.codeRepoId] ?? '',
  })),
);

// ---- bands
function bandCards(band: TimelineBand) {
  const cards = model.value?.cards;
  return band.taskIds.flatMap((id) => cards?.get(id) ?? []);
}

function bandSpans(band: TimelineBand) {
  const m = model.value;
  if (!m) return [];
  return band.spans.flatMap((s) => {
    const card = m.cards.get(s.taskId);
    if (!card) return [];
    return {
      taskId: s.taskId,
      title: card.title,
      color: card.color,
      note: t(
        s.merges
          ? 'space.ade.plan.view.spanNoteMerges.label'
          : 'space.ade.plan.view.spanNote.label',
        {
          day: s.dayNumber,
          count: s.dayCount,
        },
      ),
      merges: s.merges,
      tip: t('space.ade.plan.view.spanTip.tooltip', { day: dayLabel(m.cal, s.startDay) }),
    };
  });
}

function bandHistory(band: TimelineBand) {
  return band.history.map((h) => ({
    key: h.taskId,
    merged: !!h.mergedAt,
    meta: [
      h.mergedAt
        ? t('space.ade.plan.view.mergedArchived.status')
        : t('space.ade.panel.archived.label'),
      h.codeRepoIds.map(repoLabel).join(' · '),
    ]
      .filter(Boolean)
      .join(' · '),
    title: h.title,
  }));
}

function overdueNote(band: TimelineBand): string {
  return t('space.ade.plan.view.overdue.status', { count: band.overdueTaskIds.length });
}

function overflowNote(band: TimelineBand): string {
  return t('space.ade.plan.view.overflow.status', {
    hours: band.overflowHours,
    limit: settings.value.workdayHours,
  });
}

function overflowLabel(band: TimelineBand): string {
  const m = model.value;
  if (!m) return '';
  const ids = band.overflowTaskIds;
  const day = dayLabel(m.cal, band.overflowMoveTo);
  return ids.length === 1
    ? t('space.ade.plan.view.overflowOne.label', {
        day,
        title: m.cards.get(ids[0] as string)?.title ?? '',
      })
    : t('space.ade.plan.view.overflowMany.label', { day, count: ids.length });
}

// ---- plan writes
async function shift(ids: readonly string[], toDay: number): Promise<void> {
  const m = model.value;
  if (!m) return;
  await setPlan.mutateAsync(shiftPlanArgs(m.board.plan, today.value, ids, toDay)).catch(() => {
    /* surfaced through setPlan.error */
  });
}

function onRollover(band: TimelineBand): void {
  const m = model.value;
  if (m) void shift(band.overdueTaskIds, firstWork(m.cal, 0));
}

function onOverflowMove(band: TimelineBand): void {
  void shift(band.overflowTaskIds, band.overflowMoveTo);
}

const drag = usePlanDrag((taskId, target) => {
  const m = model.value;
  if (!m) return;
  const verdict = dropVerdict(m.view, m.board.plan, today.value, taskId, target);
  if (verdict.kind === 'move') setPlan.mutate(verdict.args);
});
const dragOverDay = computed(() =>
  drag.draggedId.value && drag.target.value?.kind === 'band' ? drag.target.value.day : null,
);

// ---- day menu
const dayOff = ref<{ title: string; text: string; yes: string; ids: string[]; to: number } | null>(
  null,
);

function startIdsOf(band: TimelineBand): string[] {
  const entries = model.value?.view.entries;
  return band.taskIds.filter((id) => entries?.get(id)?.kind === 'task');
}

function onDayMenu(band: TimelineBand, ev: MouseEvent): void {
  const m = model.value;
  if (!m) return;
  const starts = startIdsOf(band);
  const menu = dayMenuFor(m.cal, settings.value, today.value, band.key, starts.length);
  if (!menu) return;
  const items: MenuItem[] = [
    { type: 'label', label: band.label },
    {
      type: 'item',
      id: 'ade-day-toggle',
      label: menu.label,
      run: async () => {
        await settingsStore.patchSettings({ ade: menu.patch });
        if (!menu.confirm) return;
        const to = nextWork(m.cal, band.key);
        const n = starts.length;
        const toLabel = dayLabel(m.cal, to);
        dayOff.value = {
          title: t('space.ade.dayOffConfirm.title'),
          text: t('space.ade.dayOffConfirm.body', { day: band.label, count: n, to: toLabel }),
          yes: t('space.common.move.label'),
          ids: starts,
          to,
        };
      },
    },
  ];
  contextMenu.openContextMenu(ev, items);
}

const dayOffRun = useConfirmRun(
  () => dayOff.value !== null,
  async () => {
    const d = dayOff.value;
    const m = model.value;
    if (!d || !m) return null;
    await setPlan.mutateAsync(shiftPlanArgs(m.board.plan, today.value, d.ids, d.to));
    return null;
  },
  () => {
    dayOff.value = null;
  },
);
const planError = computed(() => (setPlan.isError.value ? errorText(setPlan.error.value) : ''));

// ---- scroll
function scrollToDay(k: number): void {
  const el = scrollEl.value;
  const target = el?.querySelector<HTMLElement>(`[data-ade-day="${k}"]`);
  if (!el || !target) return;
  scrollY.value =
    el.scrollTop +
    target.getBoundingClientRect().top -
    el.getBoundingClientRect().top -
    headerHeight.value -
    2;
}

async function openHistory(): Promise<void> {
  const el = scrollEl.value;
  if (!el) return;
  const before = el.scrollHeight;
  ui.showHistory = true;
  await nextTick();
  scrollY.value = el.scrollHeight - before - 60;
}

function hideHistory(): void {
  ui.showHistory = false;
  ui.historyReach = null;
  scrollY.value = 0;
}

const inHistory = computed(() => {
  if (!ui.showHistory) return false;
  const el = scrollEl.value;
  const todayBand = el?.querySelector<HTMLElement>('[data-ade-day="0"]');
  if (!el || !todayBand) return false;
  const todayTop =
    todayBand.getBoundingClientRect().top - el.getBoundingClientRect().top + el.scrollTop;
  return scrollY.value + headerHeight.value < todayTop - 30;
});

async function revealDay(k: number): Promise<void> {
  if (k < 0) {
    ui.historyReach = Math.max(ui.historyReach ?? settingsStore.ade.historyDays, -k + 2);
    await nextTick();
    scrollToDay(k);
    return;
  }
  await nextTick();
  scrollToDay(k < WINDOW_DAYS ? k : LATER);
}

async function onGoToDate(iso: string): Promise<void> {
  await revealDay(isoToOffset(today.value, iso));
}

async function onFindArchived(taskId: string): Promise<void> {
  ui.select(taskId);
  const item = model.value?.view.history.find((h) => h.taskId === taskId);
  if (!item) return;
  ui.showHistory = true;
  await revealDay(item.day);
}

const forcePushTarget = ref<{ branchId: string; branchName: string; repo: string } | null>(null);

function onForcePush(row: BranchRowModel): void {
  forcePushTarget.value = { branchId: row.id, branchName: row.name, repo: row.repo };
}
</script>

<template>
  <div
    ref="scrollEl"
    :class="cn(paneFrame, 'relative flex-1 overflow-auto bg-chrome')"
    data-testid="ade-plan"
  >
    <QueryState
      :query="boardQuery"
      :is-empty="() => false"
      :loading-label="t('space.ade.plan.view.loading.loading')"
    >
      <template v-if="model">
        <div ref="headerEl" class="sticky top-0 z-2 bg-chrome">
          <AdePlanRepos
            :repos="chips"
            :all-pending="refreshingAll"
            :ripple="model.ripple"
            @refresh-all="runRefresh([], true)"
            @refresh-one="(id) => runRefresh([id], false)"
            @toggle="(id) => ui.toggleRepo(id)"
          />
          <AdeHistoryBar
            v-if="inHistory"
            @go-to-date="onGoToDate"
            @hide="hideHistory"
            @current="scrollToDay(0)"
          />
        </div>
        <div class="px-4 pb-4 pt-3">
          <div v-if="planError" class="mb-2">
            <Alert variant="error" data-testid="ade-plan-error">
              <AlertDescription>{{ planError }}</AlertDescription>
              <AlertAction>
                <Button variant="link" size="kira" @click="setPlan.reset()">{{
                  t('common.action.dismiss.label')
                }}</Button>
              </AlertAction>
            </Alert>
          </div>
          <AdeHistoryControl
            :count="model.view.historyCount"
            :tip="model.view.historyTip"
            :open="ui.showHistory"
            @load="openHistory"
            @hide="hideHistory"
            @pick="onFindArchived"
          />
          <template v-for="band in model.view.bands" :key="band.key">
            <AdeDayBand
              :band="band"
              :cards="bandCards(band)"
              :spans="bandSpans(band)"
              :history="bandHistory(band)"
              :overdue-note="overdueNote(band)"
              :overflow-note="overflowNote(band)"
              :overflow-label="overflowLabel(band)"
              :drag-over="dragOverDay === band.key"
              @select="(id) => ui.select(id)"
              @rollover="onRollover(band)"
              @overflow-move="onOverflowMove(band)"
              @day-menu="(ev) => onDayMenu(band, ev)"
              @force-push="onForcePush"
              @drag-start="drag.begin"
              @drag-end="drag.end"
            />
          </template>
          <div v-if="model.view.hiddenCount > 0" class="ml-15 mt-2">
            <Button
              variant="toolbar"
              size="kira-lg"
              class="w-full justify-center text-muted-foreground"
              data-testid="ade-load-all"
              @click="ui.showAllItems = true"
            >
              <CodiconIcon name="arrow-down" :size="12" />
              <span>{{ model.view.moreButtonLabel }}</span>
            </Button>
          </div>
          <div v-if="model.view.canCollapse" class="ml-15 mt-2">
            <Button
              variant="toolbar"
              size="kira-lg"
              class="w-full justify-center text-muted-foreground"
              data-testid="ade-collapse"
              @click="ui.showAllItems = false"
            >
              {{ t('space.ade.plan.view.collapse.label', { count: ITEM_LIMIT }) }}
            </Button>
          </div>
        </div>
      </template>
    </QueryState>
    <ConfirmDialogView
      :open="dayOff !== null"
      :title="dayOff?.title ?? ''"
      :message="dayOff?.text ?? ''"
      :confirm-label="dayOff?.yes ?? ''"
      tone="primary"
      :pending="dayOffRun.busy.value"
      :error="dayOffRun.error.value ?? ''"
      :testids="{
        root: 'ade-confirm-dialog',
        cancel: 'ade-confirm-no',
        confirm: 'ade-confirm-yes',
        error: 'ade-confirm-error',
      }"
      @confirm="dayOffRun.confirm"
      @cancel="dayOff = null"
    />
    <AdeForcePushDialog :target="forcePushTarget" @close="forcePushTarget = null" />
  </div>
</template>
