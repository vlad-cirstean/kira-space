<script setup lang="ts">
import AdeActivityIcon from '@ade/AdeActivityIcon.vue';
import AdeChip from '@ade/AdeChip.vue';
import { activityKind, activityLabel } from '@ade/activity';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { Progress } from '@theme/components/ui/progress';
import { Textarea } from '@theme/components/ui/textarea';
import { formatElapsed, formatPercent, formatRelative } from '@theme/format';
import { t } from '@theme/i18n';
import ConfirmDialogView from '@workbench/components/ConfirmDialogView.vue';
import { storeToRefs } from 'pinia';
import { computed } from 'vue';
import PhoneBranchRow from '../components/PhoneBranchRow.vue';
import PhoneRunRow from '../components/PhoneRunRow.vue';
import { useAuthStore } from '../state/auth';
import type { PlanCard } from '../state/useAgentsModel';
import { useAgentsModel } from '../state/useAgentsModel';
import { usePhoneRuns } from '../state/usePhoneStatus';
import { useTaskActions } from '../state/useTaskActions';
import PlanTaskPrs from './PlanTaskPrs.vue';

// One planned task: status, current step and progress, expanding to every step.
const props = defineProps<{ card: PlanCard; open: boolean }>();
defineEmits<{ toggle: [] }>();

const { permissions } = storeToRefs(useAuthStore());
const actions = useTaskActions(() => props.card);
const canMove = computed(() => permissions.value.write && props.card.blocks.length > 0);
const stopFirst = computed(() => props.card.moves.live);
const { nowMs } = useAgentsModel();
const runsQuery = usePhoneRuns();
const taskRuns = computed(() =>
  (runsQuery.data.value ?? []).filter((r) => r.taskId === props.card.task.id),
);

const SESSIONS_TIP = {
  input: 'space.ade.plan.card.sessionsInput.tooltip',
  unseen: 'space.ade.plan.card.sessionsUnseen.tooltip',
  finished: 'space.ade.plan.card.sessionsFinished.tooltip',
} as const;
const sessionsTip = computed(() => {
  const s = props.card.sessions;
  if (!s) return '';
  if (s.state === 'running')
    return t('space.ade.plan.card.sessionsRunning.tooltip', { count: s.running });
  return t(SESSIONS_TIP[s.state]);
});
const runLine = computed(() => {
  const run = props.card.run;
  if (!run) return '';
  const end = run.finishedAt ?? (run.state === 'running' ? nowMs.value : null);
  if (run.startedAt === null || end === null) return run.label;
  return t('common.dotPair.label', {
    first: run.label,
    second: formatElapsed(end - run.startedAt),
  });
});
</script>

<template>
  <li class="border-b border-border" data-testid="plan-task" :data-status="card.status.id">
    <button
      type="button"
      class="flex w-full items-start gap-3 bg-transparent px-3 py-3 text-left text-fg"
      :aria-expanded="open"
      @click="$emit('toggle')"
    >
      <StatusDot class="mt-1.5" :color="card.color" />
      <span class="flex min-w-0 flex-1 flex-col gap-1">
        <span class="text-kira-md">{{ card.title }}</span>
        <span class="flex flex-wrap items-center gap-1.5 text-kira-sm">
          <AdeChip :label="card.status.name" :color="card.status.color" />
          <AdeChip
            v-if="card.blocked"
            :label="t('space.common.status.blocked.status')"
            tone="red"
            data-testid="plan-blocked"
          />
          <span v-if="card.progress.label" class="text-muted-foreground" data-testid="plan-step">{{
            card.progress.label
          }}</span>
          <AdeChip v-for="repo in card.repos" :key="repo" :label="repo" tone="grey" />
          <AdeChip v-if="card.jira" :label="card.jira" tone="blue" data-testid="plan-jira" />
          <AdeChip
            v-if="card.branchRequest"
            :label="t('space.ade.taskTab.branchRequest.label')"
            tone="amber"
            data-testid="plan-branch-request"
          />
        </span>
        <PlanTaskPrs :branch-ids="card.task.branchIds" />
        <span
          v-if="card.sessions || card.run"
          class="flex flex-wrap items-center gap-x-3 gap-y-1 text-kira-sm"
        >
          <span
            v-if="card.sessions"
            class="inline-flex items-center gap-1 text-muted-foreground"
            role="img"
            :aria-label="sessionsTip"
            :data-state="card.sessions.state"
            data-testid="plan-sessions"
          >
            <AdeActivityIcon v-if="card.sessions.state === 'input'" kind="input" :size="14" bare />
            <AdeActivityIcon
              v-else-if="card.sessions.state === 'unseen'"
              kind="unseen"
              :size="14"
              bare
            />
            <AdeActivityIcon
              v-else-if="card.sessions.state === 'running'"
              kind="working"
              :size="14"
              bare
            />
            <CodiconIcon v-else name="check" :size="13" class="text-ok" />
            <span v-if="card.sessions.running > 1" class="tabular-nums">{{
              card.sessions.running
            }}</span>
          </span>
          <span
            v-if="card.run"
            :class="
              card.run.state === 'failed' || card.run.state === 'stuck'
                ? 'text-error'
                : 'text-muted-foreground'
            "
            :data-state="card.run.state"
            data-testid="plan-run"
            >{{
              runLine
            }}</span
          >
        </span>
        <span
          v-if="card.run?.reason"
          class="text-kira-sm text-error"
          data-testid="plan-run-reason"
          >{{
            card.run.reason
          }}</span
        >
        <span v-if="card.progress.showBar" class="flex items-center gap-2">
          <Progress :model-value="card.progress.percent" />
          <span class="text-kira-sm text-fg">{{ formatPercent(card.progress.percent) }}</span>
        </span>
        <span v-if="card.attention" class="text-kira-sm text-warn" data-testid="plan-attention">{{
          card.attention
        }}</span>
      </span>
    </button>

    <ol
      v-if="open"
      class="m-0 flex list-none flex-col gap-2 px-3 pb-3 pl-8"
      data-testid="plan-steps"
    >
      <li v-if="!card.blocks.length" class="text-kira-sm text-subtle">{{
        t('mobile.plan.noWorkflow.empty')
      }}</li>
      <li
        v-for="block in card.blocks"
        :key="block.step.id"
        class="flex items-center gap-2 text-kira-sm"
        :data-state="block.state"
      >
        <StatusDot :color="`var(--color-tone-${block.tone}-solid)`" />
        <span
          class="min-w-0 flex-1 truncate"
          :class="block.state === 'now' ? 'font-medium text-fg' : 'text-muted-foreground'"
          >{{
            block.step.name
          }}</span
        >
        <span class="text-subtle">{{ block.statusText }}</span>
      </li>
    </ol>

    <div
      v-if="open && canMove"
      class="flex flex-wrap gap-2 px-3 pb-3 pl-8"
      data-testid="plan-actions"
    >
      <Button
        size="touch"
        variant="dialog"
        :disabled="stopFirst || !card.moves.prev"
        data-testid="plan-step-back"
        @click="actions.ask('back')"
      >
        {{ t('common.action.back.label') }}
      </Button>
      <Button
        size="touch"
        variant="dialog"
        :disabled="stopFirst || !card.moves.next"
        data-testid="plan-step-next"
        @click="actions.ask('next')"
      >
        {{ t('common.action.next.label') }}
      </Button>
      <Button
        size="touch"
        v-if="actions.startKind.value"
        variant="dialog-primary"
        data-testid="plan-start"
        @click="actions.ask('start')"
      >
        {{ t('common.action.start.label') }}
      </Button>
      <FieldDescription v-if="stopFirst" class="w-full">{{
        t('mobile.plan.stopFirst.hint')
      }}</FieldDescription>
    </div>

    <div v-if="open && card.sessionViews.length" class="flex flex-col gap-1 px-3 pb-3 pl-8">
      <SectionHeading flush :label="t('space.common.sessions.label')" />
      <ul class="m-0 flex list-none flex-col gap-1 p-0" data-testid="plan-session-list">
        <li
          v-for="view in card.sessionViews"
          :key="view.session.id"
          class="flex items-center gap-2 text-kira-sm"
          data-testid="plan-session"
          :data-kind="view.kind"
        >
          <AdeActivityIcon :kind="view.kind" :size="14" bare />
          <span class="min-w-0 flex-1 truncate text-muted-foreground">{{
            t('space.statusBar.agents.rowMeta.label', {
              where: view.tabName,
              activity: activityLabel(view.kind),
              ago: formatRelative(view.session.lastActiveAt, nowMs),
            })
          }}</span>
          <Button
            v-if="
              permissions.agentInput &&
              view.session.mode === 'tui' &&
              view.session.state === 'running'
            "
            size="touch"
            as-child
            variant="dialog"
            class="no-underline"
          >
            <RouterLink
              :to="{ name: 'terminal', params: { sessionId: view.session.id } }"
              class="inline-flex items-center gap-2"
              data-testid="plan-terminal"
              :data-activity="activityKind(view.session)"
            >
              <AdeActivityIcon :kind="activityKind(view.session)" bare />
              {{ t('common.module.terminal.label') }}</RouterLink
            >
          </Button>
        </li>
      </ul>
    </div>

    <div v-if="open && card.branches.length" class="flex flex-col gap-1 px-3 pb-3 pl-8">
      <SectionHeading flush :label="t('common.module.branches.label')" />
      <ul class="m-0 flex list-none flex-col gap-2 p-0" data-testid="plan-branches">
        <PhoneBranchRow v-for="branch in card.branches" :key="branch.id" :branch="branch" />
      </ul>
    </div>

    <div v-if="open && taskRuns.length" class="flex flex-col pb-3 pl-5">
      <SectionHeading :label="t('common.scriptRuns.label')" />
      <ul class="m-0 flex list-none flex-col p-0" data-testid="plan-runs">
        <PhoneRunRow v-for="run in taskRuns" :key="run.id" :run="run" />
      </ul>
    </div>
    <ConfirmDialogView
      touch
      tone="primary"
      :open="actions.intent.value !== null"
      :title="actions.dialog.value.title"
      :message="actions.dialog.value.text"
      :confirm-label="actions.dialog.value.confirmLabel"
      :pending="actions.busy.value"
      :error="actions.error.value"
      :testids="{
        root: 'confirm-dialog',
        close: 'confirm-close',
        confirm: 'confirm-yes',
        error: 'confirm-error',
      }"
      @cancel="actions.cancel"
      @confirm="actions.confirm"
    >
      <Textarea
        v-if="actions.intent.value === 'back' || actions.intent.value === 'next'"
        v-model="actions.notes.value"
        :placeholder="t('mobile.plan.notes.label')"
        :aria-label="t('mobile.plan.notes.label')"
        data-testid="plan-step-notes"
      />
    </ConfirmDialogView>
  </li>
</template>
