<script setup lang="ts">
import type { ProofDiff, ProofStepMeta } from '@shared/domain/proof';
import EmptyState from '@theme/components/EmptyState.vue';
import { ResizeHandle } from '@theme/components/ui/resizable';
import { useLiveSize } from '@theme/components/ui/resizable/useLiveSize';
import { t } from '@theme/i18n';
import { usePersistedState } from '@workbench/behaviour/persisted';
import QueryState from '@workbench/components/QueryState.vue';
import TabStrip from '@workbench/components/TabStrip.vue';
import { computed, useTemplateRef, watch } from 'vue';
import { useCellSelectionStore } from '../state/cellSelection';
import ProofChangePanel from './ProofChangePanel.vue';
import ProofCompare from './ProofCompare.vue';
import ProofHeader from './ProofHeader.vue';
import ProofHistory from './ProofHistory.vue';
import ProofScreenshotMenu from './ProofScreenshotMenu.vue';
import ProofStepView from './ProofStepView.vue';
import { useProofSession, useProofSessions, useProofSync } from './queries';
import { useProofWindowStore } from './state';
import { createProofTabHost, type ProofTab, proofTabId } from './tabHost';

// Three panes like the Kira Space review window: sessions (left) and the step tabs (centre).
const store = useProofWindowStore();
const cellSelection = useCellSelectionStore();
const centre = useTemplateRef<HTMLElement>('centre');
useProofSync();
const sessions = useProofSessions();
const session = useProofSession(() => store.sessionId);
const detail = computed(() => session.data.value ?? null);

// Selections belong to the steps of one session; a switch drops the old session's.
watch(
  () => store.sessionId,
  (_, old) => {
    if (old) cellSelection.clearSelectedCellsWhere((id) => id.startsWith(`proof:${old}:`));
  },
);

// A deleted or evicted session leaves the selection; fall back to the newest.
watch(
  () => [sessions.data.value, store.sessionId] as const,
  ([list, id]) => {
    if (!list) return;
    if (id && list.some((s) => s.id === id)) return;
    store.selectSession(list[0]?.id ?? null);
  },
  { immediate: true },
);

const side = computed(() => store.side ?? (detail.value?.pending ? 'pending' : 'current'));
const revision = computed(() =>
  detail.value ? (side.value === 'pending' && detail.value.pending) || detail.value.current : null,
);
const comparing = computed(() => store.compare && detail.value?.pending != null);
// Compare walks the proposed steps, then the ones the edit removes (which only the old side has).
const baseTabs = computed<ProofTab[]>(() => {
  const d = detail.value;
  const rev = revision.value;
  if (!d || !rev) return [];
  const make = (r: number, meta: ProofStepMeta): ProofTab => ({
    id: proofTabId(r, meta.ordinal),
    kind: 'step',
    state: { rev: r, meta },
    active: false,
  });
  const steps = rev.steps.map((meta) => make(rev.rev, meta));
  return comparing.value
    ? [...steps, ...d.removed.map((meta) => make(d.current.rev, meta))]
    : steps;
});
// A stale id (after accept, or a side switch) falls back to the same step number, then the first.
const activeId = computed<string | null>(() => {
  const list = baseTabs.value;
  const wanted = store.activeId;
  if (wanted && list.some((tab) => tab.id === wanted)) return wanted;
  const ordinal = wanted ? Number(wanted.split(':')[1]) : null;
  return list.find((tab) => tab.state.meta.ordinal === ordinal)?.id ?? list[0]?.id ?? null;
});
const tabs = computed(() =>
  baseTabs.value.map((tab) => ({ ...tab, active: tab.id === activeId.value })),
);
const activeTab = computed(() => tabs.value.find((tab) => tab.active) ?? null);

const DIFF_BADGE: Record<ProofDiff, { icon: string; label: () => string } | null> = {
  added: { icon: 'diff-added', label: () => t('common.added.status') },
  changed: { icon: 'diff-modified', label: () => t('common.changed.status') },
  removed: { icon: 'diff-removed', label: () => t('common.removed.status') },
  unchanged: null,
};
const host = createProofTabHost(tabs, activeId, store, (tab) => {
  const badge =
    detail.value?.pending && (side.value === 'pending' || comparing.value)
      ? DIFF_BADGE[tab.state.meta.diff]
      : null;
  return badge ? { icon: badge.icon, tooltip: badge.label() } : null;
});

const LEFT_MIN = 180;
const LEFT_MAX = 480;
const storedLeft = usePersistedState('kira.proof.leftWidth', 260);
const left = useLiveSize(
  () => Math.max(LEFT_MIN, Math.min(LEFT_MAX, storedLeft.value)),
  (w) => {
    storedLeft.value = w;
  },
);

const RIGHT_MIN = 280;
const RIGHT_MAX = 720;
const storedRight = usePersistedState('kira.proof.rightWidth', 340);
const right = useLiveSize(
  () => Math.max(RIGHT_MIN, Math.min(RIGHT_MAX, storedRight.value)),
  (w) => {
    storedRight.value = w;
  },
);
</script>

<template>
  <div class="flex h-full flex-col" data-testid="proof-window">
    <ProofHeader v-if="detail" :detail="detail" :steps="revision?.steps.length ?? 0">
      <ProofScreenshotMenu :target="centre" :tabs="tabs" :session-name="detail.name" />
    </ProofHeader>
    <div v-else class="h-titlebar shrink-0 bg-chrome" />
    <div class="flex min-h-0 flex-1 gap-0.5 bg-chrome px-1.5 pb-0.5">
      <section
        class="min-h-0 flex-none overflow-hidden rounded-kira bg-bg"
        :style="{ width: `${left.size.value}px` }"
        :aria-label="t('studio.proof.history.aria')"
        data-testid="proof-left"
      >
        <ProofHistory />
      </section>
      <ResizeHandle
        :label="t('studio.proof.resize.label')"
        data-testid="proof-resize-handle"
        :min="LEFT_MIN"
        :max="LEFT_MAX"
        v-bind="left.bind"
      />
      <section
        ref="centre"
        class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-kira bg-bg"
        :aria-label="t('studio.proof.session.aria')"
        data-testid="proof-centre"
      >
        <QueryState v-if="store.sessionId" :query="session">
          <template #default="{ data }">
            <div
              class="h-tabbar min-h-0 shrink-0 overflow-hidden bg-chrome"
              :aria-label="t('studio.proof.tabs.aria')"
              data-testid="proof-tabs"
            >
              <TabStrip :host="host" />
            </div>
            <div class="min-h-0 flex-1" data-testid="proof-body">
              <ProofCompare
                v-if="activeTab && comparing"
                :key="activeTab.id"
                :detail="data"
                :meta="activeTab.state.meta"
              />
              <ProofStepView
                v-else-if="activeTab"
                :key="activeTab.id"
                :session-id="data.id"
                :rev="activeTab.state.rev"
                :ordinal="activeTab.state.meta.ordinal"
              />
            </div>
          </template>
        </QueryState>
        <EmptyState
          v-else
          fill
          icon="checklist"
          :title="t('studio.proof.session.empty.emptyHint')"
          data-testid="proof-empty"
        />
      </section>
      <template v-if="detail?.pending">
        <ResizeHandle
          :label="t('studio.proof.resize.label')"
          data-testid="proof-change-resize-handle"
          direction="reverse"
          :min="RIGHT_MIN"
          :max="RIGHT_MAX"
          v-bind="right.bind"
        />
        <section
          class="flex min-h-0 flex-none flex-col overflow-hidden rounded-kira bg-bg"
          :style="{ width: `${right.size.value}px` }"
          :aria-label="t('studio.proof.change.title')"
          data-testid="proof-right"
        >
          <ProofChangePanel :detail="detail" :pending="detail.pending" />
        </section>
      </template>
    </div>
  </div>
</template>
