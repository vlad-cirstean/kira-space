<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { type MessageKey, t } from '@theme/i18n';
import SmartBadge from '@workbench/automations/smart/SmartBadge.vue';
import { errorText } from '@workbench/bridge/codedError';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeRepoTag from '../AdeRepoTag.vue';
import AdeTip from '../AdeTip.vue';
import type { Tone } from '../board/actions';
import type { StepRun } from '../board/progress';
import { WAITING_SETUP_NOTES } from '../board/setupGate';
import { nextStepId, type StepBlock } from '../board/stepBlocks';
import type { CardModel } from '../plan/usePlanModel';
import { useApprove, useResumeRun, useRetryRun } from '../queries';
import { useReviewCode } from '../review/useReviewCode';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeStageDialogStore } from '../state/adeStageDialog';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
import { ACTION_CLASS, TONE_SOLID_CLASS, TONE_TEXT_CLASS } from '../tones';
import AdeRow from './AdeRow.vue';
import AdeRowDetail from './AdeRowDetail.vue';
import AdeRunLog from './AdeRunLog.vue';
import AdeRunOutcome from './AdeRunOutcome.vue';
import AdeRunSandboxBadge from './AdeRunSandboxBadge.vue';
import AdeSetupProgress from './AdeSetupProgress.vue';
import { ADE_ROW_GLYPH, ADE_ROW_STATUS, rowMeta } from './adeRow';

// One step of the Workflow block: header (state chip, action slot, name, mode), the step row, then
// its per-repo run lines.
const props = defineProps<{ block: StepBlock; card: CardModel }>();
const ui = useAdeBoardUiStore();
const takeOver = useAdeTakeOverStore();
const approve = useApprove();
const retry = useRetryRun();
const resume = useResumeRun();
const stage = useAdeStageDialogStore();
const contextMenu = useContextMenuStore();
const reviewCode = useReviewCode();

const error = ref('');
const openLog = ref<string | null>(null);

const STATE_TONE: Record<StepBlock['state'], Tone> = {
  past: 'green',
  now: 'amber',
  next: 'grey',
  skipped: 'grey',
};
const current = computed(() => props.block.state === 'now');
const skipped = computed(() => props.block.state === 'skipped');

function moveTo(stepId: string): void {
  const taskId = props.card.task.id;
  delete ui.actionError[taskId];
  stage.open({ taskId, toStepId: stepId });
}
function onMenu(ev: MouseEvent): void {
  ev.preventDefault();
  const live = props.card.task.runs.some((r) => r.state === 'running');
  const hint = live ? t('space.ade.board.taskMenu.liveBlocked.tooltip') : undefined;
  const items: MenuItem[] = [
    { type: 'label', label: props.block.step.name },
    {
      type: 'item',
      id: 'ade-step-move-here',
      label: t('space.ade.stepBlock.moveHere.label'),
      disabled: current.value || skipped.value || live,
      hint: current.value || skipped.value ? undefined : hint,
      run: () => moveTo(props.block.step.id),
    },
  ];
  if (current.value) {
    items.push({
      type: 'item',
      id: 'ade-step-skip-this',
      label: t('space.ade.stepBlock.skipThis.label'),
      disabled: live,
      hint,
      run: () => moveTo(nextStepId(props.card.blocks, props.block.step.id)),
    });
  }
  contextMenu.openContextMenu(ev, items);
}
const boxClass = computed(() => {
  if (current.value) return 'border-tone-amber';
  return '';
});
/** The row of a run line held back by its branch's prepare script. */
// A failed step that reported detail (reason, conflicted files, last git error) shows it.
const reportRun = (runId: string) => {
  const r = props.card.task.runs.find((x) => x.id === runId);
  return r?.outcome?.report && (r.state === 'failed' || r.state === 'stuck') ? r : undefined;
};
// Repo label per run target; the branch name too when another branch of the task shares the repo.
function target(branchId: string): {
  repo: string;
  codeRepoId: string;
  name: string;
  named: boolean;
} {
  const row = props.card.rows.find((r) => r.id === branchId);
  if (!row) {
    return {
      repo: t('space.ade.stepBlock.taskFolder.label'),
      codeRepoId: '',
      name: '',
      named: false,
    };
  }
  const shared = props.card.rows.some(
    (r) => r.id !== row.id && r.branch.codeRepoId === row.branch.codeRepoId,
  );
  return {
    repo: row.repo,
    codeRepoId: row.branch.codeRepoId,
    name: row.name,
    named: shared && row.name !== '',
  };
}
// Run lines read in the branch list's order, the task folder first; each carries its target.
const runs = computed(() => {
  const at = (branchId: string): number => props.card.rows.findIndex((r) => r.id === branchId);
  return props.block.runs
    .toSorted((a, b) => at(a.run.branchId) - at(b.run.branchId))
    .map((rl) => ({ ...rl, target: target(rl.run.branchId) }));
});
const heldRow = (rl: { run: { branchId: string }; note: string }) =>
  WAITING_SETUP_NOTES.includes(rl.note)
    ? props.card.rows.find((r) => r.id === rl.run.branchId)
    : undefined;
const STATE_LABEL = {
  past: 'common.done.status',
  now: 'common.now.status',
  next: 'common.action.next.label',
  skipped: 'common.skipped.status',
} as const satisfies Record<StepBlock['state'], MessageKey>;
const stateLabel = (s: StepBlock['state']): string => t(STATE_LABEL[s]);

const GLYPH_BOX: Record<string, string> = {
  done: 'bg-tone-green-solid text-bg',
  stuck: 'bg-tone-amber-solid text-bg',
  failed: 'bg-tone-red-solid text-bg',
  running: 'border border-tone-amber-solid text-tone-amber-solid',
  pending: 'border border-border text-subtle',
};
const STEP_GLYPH: Record<string, string> = {
  done: '✓',
  running: '●',
  stuck: '!',
  failed: '✕',
  pending: '',
};
const glyphClass = (state: string): string => GLYPH_BOX[state] ?? GLYPH_BOX.pending ?? '';
const TEXT_TONE: Record<string, string> = {
  stuck: 'text-tone-red',
  failed: 'text-tone-red',
  running: 'text-tone-amber',
  done: 'text-tone-green',
};
const statusClass = (state: string): string => TEXT_TONE[state] ?? 'text-muted-foreground';

async function run(fn: () => Promise<unknown>): Promise<void> {
  error.value = '';
  try {
    await fn();
  } catch (err) {
    error.value = errorText(err);
  }
}
const onApprove = (stepId: string): Promise<void> =>
  run(() => approve.mutateAsync({ taskId: props.card.task.id, stepId }));
const onRetry = (runId: string): Promise<void> => run(() => retry.mutateAsync({ runId }));
const onResume = (runId: string): Promise<void> => run(() => resume.mutateAsync({ runId }));

const canTakeOver = (r: StepRun): boolean =>
  props.block.step.kind === 'agent' &&
  r.sessionId !== '' &&
  (r.state === 'stuck' || r.state === 'failed' || r.state === 'running');

const onReview = (branchId: string): void => {
  error.value = '';
  reviewCode.open(props.card.task.id, branchId, null, { onError: (msg) => (error.value = msg) });
};
function logToggleLabel(runId: string): string {
  const open = openLog.value === runId;
  if (props.block.step.kind === 'script') {
    return open ? t('space.ade.stepBlock.hideOutput.label') : t('space.ade.stepBlock.output.label');
  }
  return open ? t('space.common.hideLog.label') : t('space.ade.needsPage.log.label');
}
</script>

<template>
  <Card
    variant="section"
    class="-mx-px gap-0 py-1"
    :class="[skipped ? 'text-muted-foreground' : '', boxClass]"
    data-testid="ade-step-block"
    :data-step-id="block.step.id"
    :data-state="block.state"
  >
    <AdeRow data-testid="ade-step-block-head" @contextmenu="onMenu">
      <template #lead>
        <span :class="[ADE_ROW_GLYPH, glyphClass(block.step.state)]">{{
          STEP_GLYPH[block.step.state]
        }}</span>
      </template>
      <span :class="rowMeta" class="shrink-0">{{ block.step.n }}.</span>
      <span
        class="min-w-0 truncate font-medium"
        :class="block.step.state === 'done' ? 'text-muted-foreground' : 'text-fg'"
        data-testid="ade-step-name"
        >{{
          block.step.name
        }}</span
      >
      <AdeChip :label="stateLabel(block.state)" :tone="STATE_TONE[block.state]" />
      <SmartBadge v-if="block.step.smartScript" />
      <span class="min-w-0 truncate" :class="rowMeta" data-testid="ade-step-block-mode">{{
        block.mode
      }}</span>
      <template #actions><slot name="actions" /></template>
    </AdeRow>

    <div class="flex flex-col" data-testid="ade-step" :data-step-id="block.step.id">
      <AdeRow>
        <span
          :class="[ADE_ROW_STATUS, statusClass(block.step.state)]"
          data-testid="ade-step-status"
          >{{
            block.statusText
          }}</span
        >
        <span
          v-if="block.routeText"
          data-testid="ade-step-route"
          :class="ADE_ROW_STATUS"
          class="text-tone-blue"
          >{{
            block.routeText
          }}</span
        >
        <span v-if="block.gated" :class="ADE_ROW_STATUS" class="text-tone-amber">{{
          t('space.ade.stepBlock.needsApproval.status')
        }}</span>
        <AdeTip v-if="block.scope" :text="block.scope">
          <span
            class="min-w-0 shrink truncate"
            :class="[rowMeta, block.step.kind === 'script' ? 'font-data' : '']"
            data-testid="ade-step-scope"
            >{{
              block.scope
            }}</span
          >
        </AdeTip>
        <template v-if="block.step.approval" #actions>
          <Button
            variant="dialog"
            size="kira"
            class="shrink-0"
            :class="TONE_SOLID_CLASS.amber"
            data-testid="ade-step-approve"
            @click="onApprove(block.step.id)"
          >
            {{ t('common.action.approve.label') }}
          </Button>
        </template>
      </AdeRow>
      <template v-for="rl in runs" :key="rl.run.branchId">
        <AdeRow :depth="1" data-testid="ade-run-line" :data-branch-id="rl.run.branchId">
          <template #lead>
            <span :class="[ADE_ROW_GLYPH, TONE_TEXT_CLASS[rl.tone]]" data-testid="ade-run-glyph">{{
              rl.glyph
            }}</span>
          </template>
          <AdeRepoTag :code-repo-id="rl.target.codeRepoId" :label="rl.target.repo" />
          <span
            v-if="rl.target.named"
            class="max-w-40 shrink-0 truncate font-data"
            :class="rowMeta"
            data-testid="ade-run-branch"
            >{{
              rl.target.name
            }}</span
          >
          <span :class="[ADE_ROW_STATUS, TONE_TEXT_CLASS[rl.tone]]" data-testid="ade-run-status">{{
            rl.status
          }}</span>
          <AdeRunSandboxBadge
            v-if="rl.run.sandbox && rl.run.runId"
            :run-id="rl.run.runId"
            :badge="rl.run.sandbox"
          />
          <AdeTip v-if="rl.returns" :text="rl.returnsTip">
            <span class="shrink-0" data-testid="ade-run-returns" :data-spent="rl.returnsSpent"
              ><AdeChip :label="rl.returns" :tone="rl.returnsSpent ? 'red' : 'amber'" /></span
            >
          </AdeTip>
          <AdeTip v-if="rl.note" :text="rl.note">
            <span class="min-w-0 truncate" :class="rowMeta" data-testid="ade-run-note">{{
              rl.note
            }}</span>
          </AdeTip>
          <template #actions>
            <Button
              v-if="rl.hasLog"
              variant="dialog"
              size="kira"
              class="shrink-0"
              data-testid="ade-run-log-toggle"
              @click="openLog = openLog === rl.run.runId ? null : rl.run.runId"
            >
              {{ logToggleLabel(rl.run.runId) }}
            </Button>
            <AdeTip v-if="rl.canResume" :text="t('space.ade.stepBlock.resume.tooltip')">
              <Button
                variant="dialog"
                size="kira"
                class="shrink-0"
                :class="ACTION_CLASS.agent"
                :pending="resume.isPending.value"
                data-testid="ade-run-resume"
                @click="onResume(rl.run.runId)"
              >
                {{ t('common.action.resume.label') }}
              </Button>
            </AdeTip>
            <Button
              v-if="rl.canReview"
              variant="dialog"
              size="kira"
              class="shrink-0"
              data-testid="ade-run-review"
              @click="onReview(rl.run.branchId)"
            >
              {{ t('space.ade.board.branchTag.review.label') }}
            </Button>
            <AdeTip
              v-if="canTakeOver(rl.run)"
              :text="
                rl.run.sandbox && rl.run.sandbox.access !== 'off'
                  ? t('space.ade.sandbox.takeOver.hint')
                  : ''
              "
            >
              <Button
                variant="dialog"
                size="kira"
                class="shrink-0"
                :class="ACTION_CLASS.agent"
                :pending="takeOver.pending.has(rl.run.sessionId)"
                data-testid="ade-run-takeover"
                @click="takeOver.request(rl.run.sessionId)"
              >
                {{ t('common.action.takeOver.label') }}
              </Button>
            </AdeTip>
            <Button
              v-if="rl.canRetry"
              variant="dialog"
              size="kira"
              class="shrink-0"
              :class="TONE_SOLID_CLASS.red"
              data-testid="ade-run-retry"
              @click="onRetry(rl.run.runId)"
            >
              {{ t('common.action.retry.label') }}
            </Button>
          </template>
        </AdeRow>
        <AdeRowDetail v-if="heldRow(rl)?.branch.setup" :depth="1">
          <AdeSetupProgress compact :branch="heldRow(rl)!.branch" :repo="heldRow(rl)!.repo" />
        </AdeRowDetail>
        <AdeRowDetail v-if="reportRun(rl.run.runId)" :depth="1">
          <AdeRunOutcome
            :run="reportRun(rl.run.runId)!"
            :title="t('space.ade.stepBlock.report.title', { step: block.step.name })"
          />
        </AdeRowDetail>
        <AdeRowDetail v-if="openLog === rl.run.runId && rl.run.runId" :depth="1">
          <AdeRunLog kind="run" :id="rl.run.runId" max-height="140px" />
        </AdeRowDetail>
      </template>
    </div>

    <AdeRowDetail v-if="error">
      <Alert variant="error" data-testid="ade-step-block-error"
        ><AlertDescription>{{ error }}</AlertDescription></Alert
      >
    </AdeRowDetail>
  </Card>
</template>
