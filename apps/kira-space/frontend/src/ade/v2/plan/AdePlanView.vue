<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { toast } from '@theme/components/ui/sonner';
import { t } from '@theme/i18n';
import { paneFrame } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import { useElementSize, useIntersectionObserver, useScroll } from '@vueuse/core';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { useKeptScroll } from '@workbench/behaviour/viewState';
import { errorText } from '@workbench/bridge/codedError';
import ConfirmDialogView from '@workbench/components/ConfirmDialogView.vue';
import QueryState from '@workbench/components/QueryState.vue';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, nextTick, ref } from 'vue';
import { useSettingsStore } from '../../../state/settings';
import AdeForcePushDialog from '../AdeForcePushDialog.vue';
import { dayLabel, firstWork, isoToOffset, LATER, nextWork } from '../board/calendar';
import { dropVerdict, shiftPlanArgs } from '../board/dropPlan';
import { isWorkKind } from '../board/kind';
import { dayMenuFor } from '../board/menu';
import { type ForcePushTarget, remoteErrorText } from '../board/panelFacts';
import { ITEM_LIMIT, type TimelineBand, WINDOW_DAYS } from '../board/timeline';
import { useRefresh, useSetPlan } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useConfirmRun } from '../useConfirmRun';
import AdeDayBand from './AdeDayBand.vue';
import AdeHistoryBar from './AdeHistoryBar.vue';
import AdeHistoryControl from './AdeHistoryControl.vue';
import { usePlanDrag } from './usePlanDrag';
import { type BranchRowModel, type CardModel, type PlanModel, usePlanModel } from './usePlanModel';

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
const refreshing = useAsyncAction(refreshAll);
const refreshingAll = refreshing.pending;
// Repos whose facts are past the deadline; the flag arrives on a board push (P622).
const updatingRepos = computed(
  () =>
    model.value?.board.repos.filter((r) => r.updating).map((r) => repoLabel(r.codeRepoId)) ?? [],
);
const isUpdating = computed(() => updatingRepos.value.length > 0);

/** A failed fetch shows as a toast naming its repo. */
function failRefresh(codeRepoId: string, message: string): void {
  toast.error(t('common.dashPair.label', { first: repoLabel(codeRepoId), second: message }));
}

// Failures are per-repo toasts, so the action itself never fails.
async function refreshAll(): Promise<void> {
  const ids = model.value?.board.repos.map((r) => r.codeRepoId) ?? [];
  try {
    const res = await refresh.mutateAsync({ codeRepoIds: ids });
    for (const r of res.repos) if (r.error) failRefresh(r.codeRepoId, remoteErrorText(r.error));
  } catch (err) {
    const msg = errorText(err);
    for (const id of ids) failRefresh(id, msg);
  }
}

async function runRefresh(): Promise<void> {
  await refreshing.run();
}

// ---- bands
const NO_CARDS: CardModel[] = [];
const NO_SPANS: BandProps['spans'] = [];
const NO_HISTORY: BandProps['history'] = [];

interface BandProps {
  cards: CardModel[];
  spans: {
    taskId: string;
    title: string;
    color: string;
    note: string;
    merges: boolean;
    tip: string;
  }[];
  history: {
    key: string;
    merged: boolean;
    meta: string;
    title: string;
  }[];
}

function spansOf(m: PlanModel, band: TimelineBand): BandProps['spans'] {
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

function historyOf(band: TimelineBand): BandProps['history'] {
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

// One pass per model change, not per render: a selection or hover re-renders the view and reuses
// every band's arrays, so unchanged bands skip their patch.
const bandProps = computed(() => {
  const m = model.value;
  const out = new Map<number, BandProps>();
  if (!m) return out;
  for (const band of m.view.bands) {
    out.set(band.key, {
      cards: band.taskIds.flatMap((id) => m.cards.get(id) ?? []),
      spans: spansOf(m, band),
      history: historyOf(band),
    });
  }
  return out;
});

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

const drag = usePlanDrag(scrollEl, (taskId, target) => {
  const m = model.value;
  if (!m) return;
  const verdict = dropVerdict(m.view, m.board.plan, today.value, taskId, target);
  if (verdict.kind === 'move') setPlan.mutate(verdict.args);
});
// The snap slot shows exactly where a release lands: same verdict as the write, so a refused
// target shows nothing.
const dropSlot = computed<{ day: number; before: string | null } | null>(() => {
  const m = model.value;
  const id = drag.draggedId.value;
  const target = drag.target.value;
  if (!m || !id || !target) return null;
  if (dropVerdict(m.view, m.board.plan, today.value, id, target).kind !== 'move') return null;
  if (target.kind === 'band') return { day: target.day, before: null };
  // Band key, not entry day: days past the window render in the tail band.
  const day = m.view.bands.find((b) => b.taskIds.includes(target.taskId))?.key;
  if (day === undefined) return null;
  return { day, before: m.view.entries.get(target.taskId)?.auto ? null : target.taskId };
});

// ---- day menu
const dayOff = ref<{ title: string; text: string; yes: string; ids: string[]; to: number } | null>(
  null,
);

function startIdsOf(band: TimelineBand): string[] {
  const entries = model.value?.view.entries;
  return band.taskIds.filter((id) => {
    const kind = entries?.get(id)?.kind;
    return kind !== undefined && isWorkKind(kind);
  });
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
  syncTodayBelowLine();
}

async function openHistory(): Promise<void> {
  const el = scrollEl.value;
  if (!el) return;
  const before = el.scrollHeight;
  ui.showHistory = true;
  await nextTick();
  scrollY.value = el.scrollHeight - before - 60;
  syncTodayBelowLine();
}

function hideHistory(): void {
  ui.showHistory = false;
  ui.historyReach = null;
  scrollY.value = 0;
  void nextTick(syncTodayBelowLine);
}

// Past the point where today's band top sits 30px under the sticky header: the history is on
// screen. The observer fires when that edge crosses the line; no layout read per scroll frame.
const todayTop = ref<HTMLElement | null>(null);
const todayBelowLine = ref(false);
useIntersectionObserver(
  todayTop,
  ([entry]) => {
    const root = entry?.rootBounds;
    if (entry && root) todayBelowLine.value = entry.boundingClientRect.top > root.top;
  },
  {
    root: scrollEl,
    rootMargin: computed(() => `-${Math.round(headerHeight.value + 30)}px 0px 0px 0px`),
  },
);
// The observer only reports a change of intersection: a programmatic jump that carries today's band
// from above the viewport to below it (or back) changes none, so those scrolls re-read the edge once.
function syncTodayBelowLine(): void {
  const el = scrollEl.value;
  const band = todayTop.value;
  if (!el || !band) return;
  todayBelowLine.value =
    band.getBoundingClientRect().top > el.getBoundingClientRect().top + headerHeight.value + 30;
}
const inHistory = computed(() => ui.showHistory && todayBelowLine.value);

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

const forcePushTarget = ref<ForcePushTarget | null>(null);

function onForcePush(row: BranchRowModel): void {
  forcePushTarget.value = {
    branchId: row.id,
    branchName: row.name,
    repo: row.repo,
    taskId: row.branch.taskId,
  };
}

useKeptScroll('ade:plan', scrollEl, { ready: () => model.value !== null });
</script>

<template>
  <div
    ref="scrollEl"
    :class="cn(paneFrame, '@container relative flex-1 overflow-auto bg-chrome')"
    data-testid="ade-plan"
  >
    <QueryState
      :query="boardQuery"
      :is-empty="() => false"
      :loading-label="t('space.ade.plan.view.loading.loading')"
    >
      <template v-if="model">
        <div ref="headerEl" class="sticky top-0 z-2 bg-chrome">
          <AdeHistoryControl
            :count="model.view.historyCount"
            :tip="model.view.historyTip"
            :open="ui.showHistory"
            @load="openHistory"
            @hide="hideHistory"
            @pick="onFindArchived"
          >
            <TooltipIconButton
              icon="refresh"
              :label="
                isUpdating
                  ? t('space.ade.plan.refreshAll.updating.tooltip', {
                      repos: updatingRepos.join(', '),
                    })
                  : t('common.refreshAll.label')
              "
              :pending="refreshingAll"
              :data-updating="isUpdating || undefined"
              data-testid="ade-refresh-all"
              @click="runRefresh()"
            />
          </AdeHistoryControl>
          <AdeHistoryBar
            v-if="inHistory"
            @go-to-date="onGoToDate"
            @hide="hideHistory"
            @current="scrollToDay(0)"
          />
        </div>
        <div class="px-3 pb-4 pt-3">
          <div v-if="planError" class="mb-2">
            <Alert variant="error" data-testid="ade-plan-error">
              <AlertDescription>{{ planError }}</AlertDescription>
              <AlertAction>
                <Button variant="link" size="inline" @click="setPlan.reset()">{{
                  t('common.action.dismiss.label')
                }}</Button>
              </AlertAction>
            </Alert>
          </div>
          <template v-for="band in model.view.bands" :key="band.key">
            <span
              v-if="band.key === 0"
              :ref="(el) => (todayTop = el as HTMLElement | null)"
              class="block h-0"
              aria-hidden="true"
            />
            <AdeDayBand
              :band="band"
              :cards="bandProps.get(band.key)?.cards ?? NO_CARDS"
              :spans="bandProps.get(band.key)?.spans ?? NO_SPANS"
              :history="bandProps.get(band.key)?.history ?? NO_HISTORY"
              :overdue-note="overdueNote(band)"
              :overflow-note="overflowNote(band)"
              :overflow-label="overflowLabel(band)"
              :drop-before="dropSlot?.day === band.key ? dropSlot.before : undefined"
              @select="(id) => ui.select(id)"
              @rollover="onRollover(band)"
              @overflow-move="onOverflowMove(band)"
              @day-menu="(ev) => onDayMenu(band, ev)"
              @force-push="onForcePush"
              @drag-start="drag.begin"
              @drag-end="drag.end"
            />
          </template>
          <div v-if="model.view.hiddenCount > 0" class="mt-2">
            <Button
              variant="toolbar"
              size="kira"
              class="w-full justify-center text-muted-foreground"
              data-testid="ade-load-all"
              @click="ui.showAllItems = true"
            >
              <CodiconIcon name="arrow-down" :size="13" />
              <span>{{ model.view.moreButtonLabel }}</span>
            </Button>
          </div>
          <div v-if="model.view.canCollapse" class="mt-2">
            <Button
              variant="toolbar"
              size="kira"
              class="w-full justify-center text-muted-foreground"
              data-testid="ade-collapse"
              @click="ui.showAllItems = false"
              ><CodiconIcon name="fold" :size="13" />
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
        close: 'ade-confirm-close',
        confirm: 'ade-confirm-yes',
        error: 'ade-confirm-error',
      }"
      @confirm="dayOffRun.confirm"
      @cancel="dayOff = null"
    />
    <AdeForcePushDialog :target="forcePushTarget" @close="forcePushTarget = null" />
  </div>
</template>
