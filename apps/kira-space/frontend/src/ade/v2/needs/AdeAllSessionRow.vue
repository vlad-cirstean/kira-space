<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useRelativeTime } from '@workbench/behaviour/format';
import Identifier from '@workbench/components/Identifier.vue';
import { computed } from 'vue';
import AdeActivityIcon from '../AdeActivityIcon.vue';
import { activityLabel, shortId } from '../activity';
import type { SessionView } from '../sessions/sessionView';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
import { ACTION_CLASS } from '../tones';
import { useNeedsAction } from './useNeedsAction';

// One session row of All sessions (mockup `agents`): glyph, state, age, button, id, repo, step and
// where it runs. A click opens it in its Sessions tab.
const props = defineProps<{ view: SessionView; archived: boolean }>();

const takeOver = useAdeTakeOverStore();
const { openSession } = useNeedsAction();
const ago = useRelativeTime(() => props.view.session.lastActiveAt);

const running = computed(() => props.view.session.state === 'running');
const input = computed(() => running.value && props.view.kind === 'input');
const state = computed(() => {
  if (props.archived) return t('space.ade.allSessions.archived.status');
  if (props.view.headless && input.value) return t('space.ade.allSessions.stuck.status');
  return activityLabel(props.view.kind);
});
const stateClass = computed(() => {
  if (!running.value) return 'text-muted-foreground';
  if (props.view.kind === 'input') return 'text-tone-amber';
  if (props.view.kind === 'working') return 'text-tone-green';
  return props.view.kind === 'waiting' ? 'text-tone-blue' : 'text-muted-foreground';
});
/** A live interactive session opens; everything else is taken over. */
const opens = computed(() => running.value && !props.view.headless);

function open(): void {
  const s = props.view.session;
  void openSession(s.id, s.taskId, s.branchId);
}
</script>

<template>
  <!-- biome-ignore lint/a11y/useSemanticElements: the row holds a button; a button cannot nest one. -->
  <div
    role="button"
    tabindex="0"
    class="flex items-center gap-3 rounded-kira px-3 py-1.5"
    data-testid="ade-all-session"
    :data-session-id="view.session.id"
    @click="open"
    @keydown.enter.self="open"
    @keydown.space.self.prevent="open"
  >
    <AdeActivityIcon :kind="view.kind" :size="14" class="w-5 shrink-0" />
    <span
      class="w-32.5 shrink-0 truncate text-kira-md"
      :class="stateClass"
      data-testid="ade-all-state"
      >{{
        state
      }}</span
    >
    <span class="w-19 shrink-0 text-kira-md text-muted-foreground">{{ ago }}</span>
    <Button
      v-if="!archived"
      size="kira-lg"
      variant="dialog"
      class="w-23 shrink-0 "
      :class="opens ? undefined : ACTION_CLASS.agent"
      :pending="takeOver.pending.has(view.session.id)"
      data-testid="ade-all-action"
      @click.stop="opens ? open() : takeOver.request(view.session.id)"
    >
      {{ opens ? t('common.action.open.label') : t('common.action.takeOver.label') }}
    </Button>
    <span v-else class="w-23 shrink-0" />
    <Identifier
      :value="view.session.id"
      :short="shortId"
      class="w-22.5 shrink-0 text-kira-sm text-muted-foreground"
    />
    <span class="w-16 shrink-0 truncate text-kira-sm text-subtle">{{
      view.repo || t('space.ade.needs.scope.spec.label')
    }}</span>
    <div class="flex min-w-0 flex-1 flex-col">
      <span class="truncate text-kira-md">{{ view.allLabel }}</span>
      <span class="truncate font-data text-kira-sm text-subtle"
        >{{ view.branch }} {{ view.worktree }}</span
      >
    </div>
  </div>
</template>
