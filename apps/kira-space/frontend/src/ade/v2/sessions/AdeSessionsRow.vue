<script setup lang="ts">
import HighlightedText from '@theme/components/HighlightedText.vue';
import Spinner from '@theme/components/Spinner.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { CommandItem } from '@theme/components/ui/command';
import { Kbd } from '@theme/components/ui/kbd';
import { t } from '@theme/i18n';
import { rowActions, rowMeta } from '@theme/lib/rowVariants';
import { useRelativeTime } from '@workbench/behaviour/format';
import { splitMatches } from '@workbench/behaviour/match';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed, ref, useTemplateRef } from 'vue';
import AdeActivityIcon from '../AdeActivityIcon.vue';
import { activityLabel } from '../activity';
import AdeRunLog from '../panel/AdeRunLog.vue';
import { runStateLabel } from '../runState';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
import AdeSessionBadge from './AdeSessionBadge.vue';
import type { SessionView } from './sessionView';
import { useMarkSeenOnView } from './useMarkSessionSeen';
import { useSessionMenu } from './useSessionMenu';

// One session of the dropdown (two lines): glyph, badge, name, age; state, where, short id. Enter or
// click runs the primary action (open / Take over / Resume); the log toggle and More actions sit
// beside the option, never inside it.
const props = defineProps<{
  view: SessionView;
  /** Lower-cased filter text, for the match marks. */
  query: string;
  archived?: boolean;
  selected?: boolean;
}>();
const emit = defineEmits<{ primary: [] }>();

const takeOver = useAdeTakeOverStore();
const menu = useSessionMenu();
const ago = useRelativeTime(() => props.view.session.lastActiveAt);
const logOpen = ref(false);
const moreBtn = useTemplateRef<{ $el: HTMLElement }>('moreBtn');

const id = computed(() => props.view.session.id);
const running = computed(() => props.view.session.state === 'running');
// A stopped session is viewed once its row is revealed (an open from the status bar or Needs page)
// or its finished run's log is open.
useMarkSeenOnView(
  () => props.view.session.id,
  () => props.view.session.unseen,
  () => logOpen.value || (!!props.selected && !running.value),
);
const hasLog = computed(() => props.view.headless && !!props.view.session.runId);
const pending = computed(() => takeOver.pending.has(id.value));
const runState = computed(() => props.view.run?.state ?? '');
const warn = computed(() =>
  running.value
    ? props.view.kind === 'input'
    : runState.value === 'stuck' || runState.value === 'failed',
);
const state = computed(() =>
  !running.value && props.view.headless && props.view.run
    ? runStateLabel(runState.value)
    : activityLabel(props.view.kind),
);
const where = computed(() => props.view.branch || t('space.ade.needs.scope.spec.label'));
const hint = computed(() => {
  if (running.value) return t('common.action.open.label');
  if (!props.archived) return t('common.action.takeOver.label');
  return props.view.session.agentSessionId ? t('common.action.resume.label') : '';
});
const logLabel = computed(() =>
  logOpen.value ? t('space.common.hideLog.label') : t('space.common.showLog.label'),
);

const marks = (text: string) => splitMatches(text, props.query);

function toggleLog(): void {
  logOpen.value = !logOpen.value;
}
const extra = computed<MenuItem[]>(() =>
  hasLog.value
    ? [
        {
          type: 'item',
          id: 'ade-session-log-menu',
          label: logLabel.value,
          icon: 'output',
          run: toggleLog,
        },
      ]
    : [],
);

function onSelect(ev: Event): void {
  ev.preventDefault();
  if (!hint.value && hasLog.value) toggleLog();
  else emit('primary');
}
function onContext(ev: MouseEvent): void {
  menu.open(ev, props.view, { archived: props.archived, extra: extra.value });
}
function openMenu(el?: Element): void {
  const anchor = el ?? moreBtn.value?.$el;
  if (anchor) menu.openFor(anchor, props.view, { archived: props.archived, extra: extra.value });
}
defineExpose({ openMenu });
</script>

<template>
  <div class="group relative">
    <CommandItem
      :value="id"
      class="peer/item group/row items-start gap-0.5 py-1 pr-16"
      data-testid="ade-sessions-row"
      :data-session-id="id"
      :data-selected="selected || undefined"
      @select="onSelect"
      @contextmenu="onContext"
    >
      <span class="flex min-w-0 flex-1 flex-col gap-0.5"
        ><span class="flex min-w-0 items-center gap-1.5">
          <AdeActivityIcon :kind="view.kind" bare />
          <AdeSessionBadge :view="view" />
          <span class="min-w-0 flex-1 truncate font-data"
            ><HighlightedText :parts="marks(view.tabName)" /></span
          >
          <span class="shrink-0" :class="rowMeta">{{ ago }}</span>
        </span>
        <span class="flex min-w-0 items-center gap-1.5" :class="rowMeta">
          <span
            class="shrink-0"
            :class="warn && 'text-tone-amber'"
            data-testid="ade-sessions-row-state"
            >{{
              state
            }}</span
          >
          <span class="min-w-0 truncate"
            ><HighlightedText :parts="marks(where)" />
            · <HighlightedText :parts="marks(view.shortId)" /></span
          >
          <Spinner v-if="pending" :size="12" label="" />
          <Kbd
            v-else-if="hint"
            class="ml-auto hidden group-data-highlighted/row:inline-flex"
            data-testid="ade-sessions-row-hint"
            >{{
              hint
            }}</Kbd
          >
        </span></span
      >
    </CommandItem>
    <span
      class="absolute right-1.5 top-1 flex items-center gap-0.5 peer-data-highlighted/item:opacity-100"
      :class="rowActions"
    >
      <TooltipIconButton
        v-if="hasLog"
        icon="output"
        :label="logLabel"
        :pressed="logOpen"
        tabindex="-1"
        data-testid="ade-sessions-row-log"
        :data-session-id="id"
        @click="toggleLog"
      />
      <TooltipIconButton
        ref="moreBtn"
        icon="ellipsis"
        :label="t('common.moreActions.tooltip')"
        tabindex="-1"
        data-testid="ade-sessions-row-more"
        :data-session-id="id"
        @click="openMenu()"
      />
    </span>
  </div>
  <AdeRunLog v-if="logOpen" kind="run" :id="view.session.runId" max-height="160px" />
</template>
