<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { type MessageKey, t } from '@theme/i18n';
import { compareNatural } from '@workbench/behaviour/match';
import { computed, ref } from 'vue';
import AdeActivityIcon from '../AdeActivityIcon.vue';
import { ACTIVITY_RANK, type ActivityKind } from '../activity';
import { usePlanModel } from '../plan/usePlanModel';
import type { SessionView } from '../sessions/sessionView';
import { useSessionViews } from '../sessions/useSessionViews';
import AdeAllSessionRow from './AdeAllSessionRow.vue';

// Every session, grouped by task: Running or Stopped, most urgent group first (SPEC2 section 11).
const { model } = usePlanModel();
const { sessions, view } = useSessionViews();

const SUMMARY = {
  input: 'space.ade.allSessions.summaryInput.label',
  working: 'space.ade.allSessions.summaryWorking.label',
  waiting: 'space.ade.allSessions.summaryWaiting.label',
} as const satisfies Partial<Record<ActivityKind, MessageKey>>;

const filter = ref<'running' | 'stopped'>('running');
const running = computed(() => sessions.value.filter((s) => s.state === 'running').length);
const stopped = computed(() => sessions.value.length - running.value);
const filterItems = computed(() => [
  {
    value: 'running',
    label: t('common.state.running.status'),
    count: running.value,
    testid: 'ade-all-running',
  },
  {
    value: 'stopped',
    label: t('space.ade.activity.stopped.status'),
    count: stopped.value,
    testid: 'ade-all-stopped',
  },
]);

interface Group {
  taskId: string;
  title: string;
  color: string;
  archived: boolean;
  rows: SessionView[];
}

function describe(taskId: string): Pick<Group, 'title' | 'color' | 'archived'> {
  const m = model.value;
  const card = m?.cardFor(taskId);
  if (card) return { title: card.title, color: card.color, archived: false };
  const entry = m?.board.history.find((h) => h.taskId === taskId);
  return { title: entry?.title || taskId, color: 'var(--kira-fg-subtle)', archived: true };
}

const groups = computed((): Group[] => {
  const want = filter.value === 'running';
  const byTask = new Map<string, SessionView[]>();
  for (const s of sessions.value) {
    if ((s.state === 'running') !== want) continue;
    const list = byTask.get(s.taskId) ?? [];
    list.push(view(s));
    byTask.set(s.taskId, list);
  }
  const rank = (rows: SessionView[]): number => Math.min(...rows.map((r) => ACTIVITY_RANK[r.kind]));
  return [...byTask.entries()]
    .map(([taskId, rows]) => ({
      taskId,
      ...describe(taskId),
      rows: rows.sort(
        (a, b) =>
          ACTIVITY_RANK[a.kind] - ACTIVITY_RANK[b.kind] ||
          compareNatural(a.session.id, b.session.id),
      ),
    }))
    .sort(
      (a, b) =>
        rank(a.rows) - rank(b.rows) ||
        compareNatural(a.title, b.title) ||
        compareNatural(a.taskId, b.taskId),
    );
});

const summary = computed(() => {
  if (filter.value !== 'running') return [];
  const counts = new Map<ActivityKind, number>();
  for (const s of sessions.value) {
    if (s.state !== 'running') continue;
    const k = view(s).kind;
    counts.set(k, (counts.get(k) ?? 0) + 1);
  }
  return (['input', 'working', 'waiting'] as const)
    .filter((k) => counts.has(k))
    .map((k) => ({ kind: k, text: t(SUMMARY[k], { count: counts.get(k) ?? 0 }) }));
});
</script>

<template>
  <div class="flex flex-col gap-4 p-3" data-testid="ade-all-sessions">
    <div class="flex items-center gap-3">
      <SecondaryTabs
        variant="segmented"
        size="kira-lg"
        :model-value="filter"
        :items="filterItems"
        count-badge
        @update:model-value="(v) => (filter = v as 'running' | 'stopped')"
      />
      <div class="flex gap-3">
        <span
          v-for="a in summary"
          :key="a.kind"
          class="inline-flex items-center gap-1 text-kira-md"
        >
          <AdeActivityIcon :kind="a.kind" />{{ a.text }}
        </span>
      </div>
    </div>
    <section
      v-for="g in groups"
      :key="g.taskId"
      class="flex flex-col gap-0.5"
      data-testid="ade-all-group"
    >
      <div class="mb-1 flex items-center gap-2 border-b border-border px-3 pb-1.5">
        <span class="size-2.5 shrink-0 rounded-kira-sm" :style="{ background: g.color }" />
        <h3 class="m-0 truncate text-kira-lg font-medium">{{ g.title }}</h3>
      </div>
      <AdeAllSessionRow v-for="r in g.rows" :key="r.session.id" :view="r" :archived="g.archived" />
    </section>
    <InlineState v-if="groups.length === 0" kind="empty" data-testid="ade-all-empty">{{
      t('space.ade.allSessions.empty.empty')
    }}</InlineState>
  </div>
</template>
