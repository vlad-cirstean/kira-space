<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import { Command, CommandGroup, CommandInput, CommandList } from '@theme/components/ui/command';
import { t } from '@theme/i18n';
import { matchesQuery } from '@workbench/behaviour/match';
import { computed, ref, useTemplateRef } from 'vue';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
import AdeSessionsRow from './AdeSessionsRow.vue';
import type { SessionView } from './sessionView';

// Sessions in scope as a keyboard list (arrows, Home / End, Enter), Running then Finished or
// stopped. The filter field shows from `FILTER_FROM` rows; filtering is own, since a row's actions
// and log sit beside its option and the built-in item filter could not hide them. `inline` fills
// the archived panel instead of a popover.
const FILTER_FROM = 6;

const props = defineProps<{
  running?: SessionView[];
  stopped: SessionView[];
  archived?: boolean;
  inline?: boolean;
  /** Row to check and scroll to, as for a reveal. */
  selectedId?: string | null;
}>();
const emit = defineEmits<{ done: [] }>();

const tabs = useAdePanelTabsStore();
const takeOver = useAdeTakeOverStore();
const search = ref('');
const root = useTemplateRef<{ $el: HTMLElement }>('root');
const rows = new Map<string, { openMenu: (el?: Element) => void } | null>();

const total = computed(() => (props.running?.length ?? 0) + props.stopped.length);
const query = computed(() => search.value.trim().toLowerCase());

function matches(v: SessionView): boolean {
  const q = query.value;
  if (!q) return true;
  const s = v.session;
  return [v.tabName, v.branch, v.repo, v.step, v.shortId, s.agentSessionId].some((f) =>
    matchesQuery(f ?? '', q),
  );
}
const shownRunning = computed(() => (props.running ?? []).filter(matches));
const shownStopped = computed(() => props.stopped.filter(matches));
const grouped = computed(() => !!props.running?.length && !!props.stopped.length);

function primary(v: SessionView): void {
  const s = v.session;
  emit('done');
  if (s.state === 'running') tabs.selectSession(s.taskId, s.id);
  else if (props.archived) void takeOver.resumeArchived(s.taskId, s.id);
  else takeOver.request(s.id);
}

// Shift+F10 / ContextMenu key: the highlighted row's menu.
function onKeydown(ev: KeyboardEvent): void {
  if (!(ev.key === 'ContextMenu' || (ev.key === 'F10' && ev.shiftKey))) return;
  const el = root.value?.$el.querySelector<HTMLElement>('[data-highlighted][data-session-id]');
  const id = el?.dataset.sessionId;
  if (!el || !id) return;
  ev.preventDefault();
  rows.get(id)?.openMenu(el);
}
</script>

<template>
  <div v-if="inline" class="px-3 pb-1 pt-2 text-kira-sm text-muted-foreground">{{
    t('space.ade.sessionList.stopped.label')
  }}</div>
  <Command
    ref="root"
    v-model:search="search"
    ignore-filter
    :model-value="selectedId ?? ''"
    class="rounded-none! bg-transparent p-0"
    :class="inline && 'min-h-0 flex-1 px-1.5 pb-2'"
    data-testid="ade-sessions-list"
    @keydown="onKeydown"
  >
    <CommandInput
      v-if="total >= FILTER_FROM"
      :placeholder="t('space.ade.sessionList.filter.placeholder')"
      data-testid="ade-sessions-menu-filter"
    />
    <CommandList :class="inline ? 'max-h-none flex-1' : 'max-h-120'">
      <InlineState
        v-if="query && !shownRunning.length && !shownStopped.length"
        kind="empty"
        data-testid="ade-sessions-menu-empty"
        >{{
          t('common.state.noMatches.empty')
        }}</InlineState
      >
      <CommandGroup
        v-if="shownRunning.length"
        :heading="
          grouped
            ? t('space.ade.allSessions.running.label', { count: shownRunning.length })
            : undefined
        "
      >
        <AdeSessionsRow
          v-for="v in shownRunning"
          :key="v.session.id"
          :ref="(r) => rows.set(v.session.id, r as never)"
          :view="v"
          :query="query"
          :selected="v.session.id === selectedId"
          @primary="primary(v)"
        />
      </CommandGroup>
      <CommandGroup
        v-if="shownStopped.length"
        :heading="
          grouped
            ? t('space.ade.allSessions.stopped.label', { count: shownStopped.length })
            : undefined
        "
      >
        <AdeSessionsRow
          v-for="v in shownStopped"
          :key="v.session.id"
          :ref="(r) => rows.set(v.session.id, r as never)"
          :view="v"
          :query="query"
          :archived="archived"
          :selected="v.session.id === selectedId"
          @primary="primary(v)"
        />
      </CommandGroup>
    </CommandList>
  </Command>
</template>
