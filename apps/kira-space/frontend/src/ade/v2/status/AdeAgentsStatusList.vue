<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useModeStore } from '../../../state/mode';
import { ACTIVITY_RANK } from '../activity';
import { usePlanModel } from '../plan/usePlanModel';
import { useOpenSession } from '../sessions/useOpenSession';
import { useSessionViews } from '../sessions/useSessionViews';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import AdeAgentsStatusRow from './AdeAgentsStatusRow.vue';

// Popover body of the status bar agents item. Mounted only while open, so the plan model behind
// the task titles loads on demand.
const emit = defineEmits<{ close: [] }>();

const mode = useModeStore();
const ui = useAdeBoardUiStore();
const { model } = usePlanModel();
const { sessions, view } = useSessionViews();
const openSession = useOpenSession();

const rows = computed(() =>
  sessions.value
    .filter((s) => s.state === 'running')
    .map((s) => ({ session: s, view: view(s) }))
    .sort(
      (a, b) =>
        ACTIVITY_RANK[a.view.kind] - ACTIVITY_RANK[b.view.kind] ||
        b.session.lastActiveAt - a.session.lastActiveAt ||
        (a.session.id < b.session.id ? -1 : 1),
    ),
);

const titleOf = (taskId: string): string => model.value?.cardFor(taskId)?.title ?? taskId;

function open(taskId: string, sessionId: string, branchId: string): void {
  emit('close');
  mode.setMode('ade');
  void openSession(sessionId, taskId, branchId);
}

function showAll(): void {
  emit('close');
  mode.setMode('ade');
  ui.view = 'needs';
  ui.showAllSessions = true;
}
</script>

<template>
  <InlineState v-if="!model" kind="loading">{{ t('common.state.loading.loading') }}</InlineState>
  <div v-else class="flex flex-col gap-0.5">
    <AdeAgentsStatusRow
      v-for="r in rows"
      :key="r.session.id"
      :view="r.view"
      :title="titleOf(r.session.taskId)"
      @open="open(r.session.taskId, r.session.id, r.session.branchId)"
    />
    <div class="flex justify-end px-1 pt-1">
      <Button variant="link" size="inline" data-testid="status-agents-all" @click="showAll">{{
        t('space.ade.needsPage.showAll.label')
      }}</Button>
    </div>
  </div>
</template>
