<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { TooltipDisabledTrigger } from '@theme/components/ui/tooltip';
import { type MessageKey, t } from '@theme/i18n';
import SmartBadge from '@workbench/automations/smart/SmartBadge.vue';
import { errorText } from '@workbench/bridge/codedError';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeRepoTag from '../AdeRepoTag.vue';
import AdeTip from '../AdeTip.vue';
import type { Tone } from '../board/actions';
import { integrationChips } from '../board/labels';
import type { StepRun } from '../board/progress';
import { WAITING_SETUP_NOTES } from '../board/setupGate';
import { nextStepId, type StepBlock } from '../board/stepBlocks';
import { type CardModel, usePlanModel } from '../plan/usePlanModel';
import { useApprove, useResumeRun, useRetryRun } from '../queries';
import { useReviewCode } from '../review/useReviewCode';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeStageDialogStore } from '../state/adeStageDialog';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
import { ACTION_CLASS, TONE_SOLID_CLASS, TONE_TAG_CLASS, TONE_TEXT_CLASS } from '../tones';
import AdeRunLog from './AdeRunLog.vue';
import AdeRunOutcome from './AdeRunOutcome.vue';
import AdeRunSandboxBadge from './AdeRunSandboxBadge.vue';
import AdeSetupProgress from './AdeSetupProgress.vue';

// One step of the Workflow block: header (state chip, action slot, name, mode), the step row, then
// its per-repo run lines, or the branches of the Release step.
const props = defineProps<{ block: StepBlock; card: CardModel }>();
const ui = useAdeBoardUiStore();
const takeOver = useAdeTakeOverStore();
const approve = useApprove();
const retry = useRetryRun();
const resume = useResumeRun();
const stage = useAdeStageDialogStore();
const contextMenu = useContextMenuStore();
const reviewCode = useReviewCode();
const { model } = usePlanModel();
const mainNameOf = (codeRepoId: string): string =>
  model.value?.board.repos.find((r) => r.codeRepoId === codeRepoId)?.mainName ?? 'main';

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
  if (current.value) return 'border-tone-amber-solid/40 bg-tone-amber-solid/5';
  return '';
});
/** The row of a run line held back by its branch's prepare script. */
// A failed step that reported detail (reason, conflicted files, last git error) shows it.
const reportRun = (runId: string) => {
  const r = props.card.task.runs.find((x) => x.id === runId);
  return r?.outcome?.report && (r.state === 'failed' || r.state === 'stuck') ? r : undefined;
};
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
  pending: 'border border-border-strong text-subtle',
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
const statusClass = (state: string): string => TEXT_TONE[state] ?? 'text-subtle';

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

const released = computed(() =>
  props.block.release ? props.card.rows.filter((r) => r.branch.kind === 'mine' && !r.draft) : [],
);
const reviewRows = computed(() =>
  props.block.step.id === 'review' ? props.card.rows.filter((r) => r.branch.kind === 'mine') : [],
);
const reviewOff = (row: CardModel['rows'][number]): boolean => {
  const c = reviewCode.choiceOf(row);
  return reviewCode.pending.value || !c || c.disabled;
};
const onReview = (branchId: string): void => {
  error.value = '';
  reviewCode.open(props.card.task.id, branchId, null, { onError: (msg) => (error.value = msg) });
};
const mainBadge = (b: CardModel['rows'][number]['branch']): string => {
  const main = mainNameOf(b.codeRepoId);
  return b.mergedIntoMain
    ? t('space.ade.progress.stepDone.label', { name: main })
    : t('space.ade.stepBlock.mainOpen.status', { main });
};
const chipClass = (tone: 'muted' | 'stale' | 'unknown'): string =>
  tone === 'stale' ? 'text-tone-amber' : 'text-muted-foreground';
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
    class="gap-1 px-2 py-2"
    :class="[skipped ? 'text-muted-foreground' : '', boxClass]"
    data-testid="ade-step-block"
    :data-step-id="block.step.id"
    :data-state="block.state"
  >
    <!-- biome-ignore lint/a11y/noStaticElementInteractions: right-click only; the header holds nested interactive controls. -->
    <div
      class="flex min-h-6 items-center gap-2"
      data-testid="ade-step-block-head"
      @contextmenu="onMenu"
    >
      <AdeChip
        :label="stateLabel(block.state)"
        :tone="STATE_TONE[block.state]"
        class="min-w-10 text-center"
      />
      <slot name="actions" />
      <span class="shrink-0 text-kira-sm text-subtle">{{ block.step.n }}.</span>
      <span
        class="min-w-0 truncate text-kira-lg font-medium"
        :class="block.step.state === 'done' ? 'text-subtle' : 'text-fg'"
        data-testid="ade-step-name"
        >{{
          block.step.name
        }}</span
      >
      <SmartBadge v-if="block.step.smartScript" />
      <span class="min-w-0 truncate text-kira-sm text-subtle" data-testid="ade-step-block-mode">{{
        block.mode
      }}</span>
    </div>

    <div
      class="flex flex-col gap-0.5 rounded-kira-sm bg-bg px-1.5 py-1"
      data-testid="ade-step"
      :data-step-id="block.step.id"
    >
      <div class="flex min-h-control items-center gap-2">
        <span
          class="box-border flex size-4 shrink-0 items-center justify-center rounded-kira-sm text-kira-sm font-medium"
          :class="glyphClass(block.step.state)"
          >{{
            STEP_GLYPH[block.step.state]
          }}</span
        >
        <span
          class="shrink-0 whitespace-nowrap text-kira-sm"
          :class="statusClass(block.step.state)"
          data-testid="ade-step-status"
          >{{
            block.statusText
          }}</span
        >
        <Button
          variant="dialog"
          v-if="block.step.approval"
          size="kira"
          class="shrink-0 "
          :class="TONE_SOLID_CLASS.amber"
          data-testid="ade-step-approve"
          @click="onApprove(block.step.id)"
        >
          {{ t('common.action.approve.label') }}
        </Button>
        <span
          v-if="block.routeText"
          data-testid="ade-step-route"
          class="shrink-0 whitespace-nowrap text-kira-sm text-tone-blue"
          >{{
            block.routeText
          }}</span
        >
        <span v-if="block.gated" class="shrink-0 whitespace-nowrap text-kira-sm text-tone-amber">{{
          t('space.ade.stepBlock.needsApproval.status')
        }}</span>
        <AdeTip v-if="block.scope" :text="block.scope">
          <span
            class="min-w-0 shrink truncate text-kira-sm text-subtle"
            :class="block.step.kind === 'script' ? 'font-data' : ''"
            data-testid="ade-step-scope"
            >{{
              block.scope
            }}</span
          >
        </AdeTip>
      </div>
      <template v-for="rl in block.runs" :key="rl.run.branchId">
        <div
          class="flex min-h-control items-center gap-2 pl-6"
          data-testid="ade-run-line"
          :data-branch-id="rl.run.branchId"
        >
          <span
            class="w-3.5 shrink-0 text-center text-kira-sm font-medium"
            :class="TONE_TEXT_CLASS[rl.tone]"
            data-testid="ade-run-glyph"
            >{{
              rl.glyph
            }}</span
          >
          <AdeRepoTag
            :code-repo-id="card.rows.find((r) => r.id === rl.run.branchId)?.branch.codeRepoId ?? ''"
            :label="
              rl.run.branchId === ''
                ? t('space.ade.stepBlock.taskFolder.label')
                : (card.rows.find((r) => r.id === rl.run.branchId)?.repo ?? '')
            "
          />
          <span
            class="min-w-16 shrink-0 whitespace-nowrap text-kira-sm"
            :class="TONE_TEXT_CLASS[rl.tone]"
            data-testid="ade-run-status"
            >{{
              rl.status
            }}</span
          >
          <AdeRunSandboxBadge
            v-if="rl.run.sandbox && rl.run.runId"
            :run-id="rl.run.runId"
            :badge="rl.run.sandbox"
          />
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
              class="shrink-0 "
              :class="ACTION_CLASS.agent"
              :pending="takeOver.pending.has(rl.run.sessionId)"
              data-testid="ade-run-takeover"
              @click="takeOver.request(rl.run.sessionId)"
            >
              {{ t('common.action.takeOver.label') }}
            </Button>
          </AdeTip>
          <Button
            variant="dialog"
            v-if="rl.canRetry"
            size="kira"
            class="shrink-0 "
            :class="TONE_SOLID_CLASS.red"
            data-testid="ade-run-retry"
            @click="onRetry(rl.run.runId)"
          >
            {{ t('common.action.retry.label') }}
          </Button>
          <AdeTip v-if="rl.returns" :text="rl.returnsTip">
            <span class="shrink-0" data-testid="ade-run-returns" :data-spent="rl.returnsSpent"
              ><AdeChip :label="rl.returns" :tone="rl.returnsSpent ? 'red' : 'amber'" /></span
            >
          </AdeTip>
          <AdeTip v-if="rl.note" :text="rl.note">
            <span
              class="min-w-0 truncate text-kira-sm text-muted-foreground"
              data-testid="ade-run-note"
              >{{
                rl.note
              }}</span
            >
          </AdeTip>
        </div>
        <AdeSetupProgress
          v-if="heldRow(rl)?.branch.setup"
          class="my-0.5 ml-6"
          compact
          :branch="heldRow(rl)!.branch"
          :repo="heldRow(rl)!.repo"
        />
        <AdeRunOutcome
          v-if="reportRun(rl.run.runId)"
          :run="reportRun(rl.run.runId)!"
          :title="t('space.ade.stepBlock.report.title', { step: block.step.name })"
          class="my-0.5 ml-6"
        />
        <div v-if="openLog === rl.run.runId && rl.run.runId" class="my-0.5 ml-6">
          <AdeRunLog kind="run" :id="rl.run.runId" max-height="140px" />
        </div>
      </template>
    </div>

    <Button
      v-for="row in released"
      :key="row.id"
      variant="toolbar"
      size="kira-lg"
      class="w-full justify-start gap-2 bg-bg px-1.5 text-left text-fg"
      data-testid="ade-release-row"
      :data-branch-id="row.id"
      @click="ui.selectBranch(card.task.id, row.id)"
    >
      <span
        class="shrink-0 rounded-kira-sm px-1.5 py-px text-kira-sm font-medium"
        :class="TONE_TAG_CLASS[row.branch.mergedIntoMain ? 'purple' : 'grey']"
        >{{
          mainBadge(row.branch)
        }}</span
      >
      <AdeTip v-for="c in integrationChips(row.branch)" :key="c.label" :text="c.tip">
        <span class="shrink-0 text-kira-sm font-medium" :class="chipClass(c.tone)">{{
          c.label
        }}</span>
      </AdeTip>
      <AdeRepoTag :code-repo-id="row.branch.codeRepoId" :label="row.repo" />
      <span class="min-w-0 truncate font-data text-kira-md">{{ row.name }}</span>
    </Button>
    <div
      v-for="row in reviewRows"
      :key="`review-${row.id}`"
      class="flex min-h-row items-center gap-2 rounded-kira-sm bg-bg px-1.5"
      data-testid="ade-review-row"
      :data-branch-id="row.id"
    >
      <AdeRepoTag :code-repo-id="row.branch.codeRepoId" :label="row.repo" />
      <span class="min-w-0 flex-1 truncate font-data text-kira-md">{{ row.name }}</span>
      <AdeTip :parts="reviewCode.choiceOf(row)?.tip ?? []">
        <TooltipDisabledTrigger :disabled="reviewOff(row)">
          <Button
            variant="dialog"
            size="kira"
            class="shrink-0"
            :disabled="reviewOff(row)"
            data-testid="ade-review-code"
            @click="onReview(row.id)"
          >
            <CodiconIcon name="git-compare" :size="13" />
            {{ t('space.ade.board.taskMenu.reviewCode.label') }}
          </Button>
        </TooltipDisabledTrigger>
      </AdeTip>
    </div>
    <Alert v-if="error" variant="error" data-testid="ade-step-block-error"
      ><AlertDescription>{{ error }}</AlertDescription></Alert
    >
  </Card>
</template>
