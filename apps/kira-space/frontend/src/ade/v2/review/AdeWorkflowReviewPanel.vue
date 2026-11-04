<script setup lang="ts">
import type { ReviewComment } from '@kira/git-ipc';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { FieldError } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { RadioGroup, RadioGroupItem } from '@theme/components/ui/radio-group';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import ConfirmDialogView from '@workbench/components/ConfirmDialogView.vue';
import { computed, ref, watch } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeTip from '../AdeTip.vue';
import type { Tone } from '../board/actions';
import AdeRunLog from '../panel/AdeRunLog.vue';
import AdeRunOutcome from '../panel/AdeRunOutcome.vue';
import {
  useFinishReview,
  useResumeRun,
  useReviewStep,
  useSendReview,
  useStopRun,
} from '../queries';
import { reviewPhaseLabel } from '../runState';
import { TONE_SOLID_CLASS } from '../tones';
import type { ReviewPhase, ReviewWindowTarget } from '../wire';
import { useReviewComments } from './useReviewComments';

// The review step of this window's branch: comments, the route to an agent, the round in flight and
// the finish buttons. The workflow waits on the run in state `review` until Finish.
const props = defineProps<{ target: ReviewWindowTarget }>();
const emit = defineEmits<{ openFile: [path: string, origin: ReviewComment['origin']] }>();

const step = useReviewStep(() => props.target.branchId);
const state = computed(() => step.data.value ?? null);
const run = computed(() => state.value?.run ?? null);
const phase = computed<ReviewPhase>(() => run.value?.review?.phase ?? 'waiting');
const round = computed(() => state.value?.round ?? null);
const live = computed(() => phase.value === 'sent' || phase.value === 'working');
const waiting = computed(() => run.value?.state === 'review');
const roundBad = computed(
  () => round.value?.run.state === 'stuck' || round.value?.run.state === 'failed',
);

const { query, setState } = useReviewComments(() => props.target);
const comments = computed(() => query.data.value ?? []);
const openCount = computed(() => comments.value.filter((c) => c.state === 'open').length);

const route = ref<'implementer' | 'new'>('implementer');
const targetRunId = ref('');
// The user's own choice wins; until then the backend's last route and target show.
const touched = ref(false);
watch(
  state,
  (s) => {
    if (!s || touched.value) return;
    route.value = s.route;
    targetRunId.value = s.targetRunId || s.defaultTargetRunId;
  },
  { immediate: true },
);

const send = useSendReview();
const finish = useFinishReview();
const stop = useStopRun();
const resume = useResumeRun();
const error = ref('');
const note = ref('');
const confirmResult = ref<string | null>(null);
const showLog = ref(false);

const noTarget = computed(
  () => route.value === 'implementer' && (state.value?.candidates.length ?? 0) === 0,
);
const sendTip = computed(() => {
  if (!waiting.value || live.value) return t('space.ade.reviewStep.roundLive.tooltip');
  if (openCount.value === 0) return t('space.ade.reviewStep.noOpen.tooltip');
  if (noTarget.value) return t('space.ade.reviewStep.noImplementer.hint');
  return '';
});

async function act(fn: () => Promise<unknown>): Promise<boolean> {
  error.value = '';
  try {
    await fn();
    return true;
  } catch (err) {
    error.value = errorText(err);
    return false;
  }
}

function onSend(): Promise<boolean> {
  const r = run.value;
  if (!r) return Promise.resolve(false);
  return act(() =>
    send.mutateAsync({
      runId: r.id,
      route: route.value,
      targetRunId: route.value === 'implementer' ? targetRunId.value : '',
    }),
  );
}

async function doFinish(result: string): Promise<void> {
  const r = run.value;
  if (!r) return;
  if (await act(() => finish.mutateAsync({ runId: r.id, result, note: note.value.trim() }))) {
    note.value = '';
  }
}

function onFinish(result: string): void {
  if (openCount.value > 0) confirmResult.value = result;
  else void doFinish(result);
}

async function confirmFinish(): Promise<void> {
  const result = confirmResult.value;
  confirmResult.value = null;
  if (result) await doFinish(result);
}

const PHASE_TONE: Record<ReviewPhase, Tone> = {
  waiting: 'blue',
  sent: 'amber',
  working: 'amber',
  stuck: 'red',
  done: 'green',
};

function lineLabel(c: ReviewComment): string {
  const r = c.range;
  return `${c.path}:${r.start === r.end ? r.start : `${r.start}-${r.end}`}`;
}

const kindLabel = (c: ReviewComment): string =>
  c.kind === 'question'
    ? t('space.ade.needs.kind.question.label')
    : t('git.review.comments.kind.change.label');
const stateLabel = (c: ReviewComment): string =>
  c.state === 'addressed'
    ? t('git.review.comments.state.addressed.label')
    : t('common.action.open.label');
</script>

<template>
  <div
    v-if="run"
    class="flex h-full min-h-0 flex-col gap-2 overflow-y-auto p-2 text-kira-md"
    data-testid="ade-review-workflow"
  >
    <div class="flex flex-wrap items-center gap-2">
      <span data-testid="ade-review-step-name">{{ state?.stepName }}</span>
      <AdeChip
        :label="reviewPhaseLabel(phase)"
        :tone="PHASE_TONE[phase]"
        data-testid="ade-review-phase"
      />
      <span
        v-if="round"
        class="text-kira-sm text-muted-foreground"
        data-testid="ade-review-round"
        >{{
          t('space.ade.reviewStep.round.label', { n: round.n })
        }}</span
      >
    </div>

    <p v-if="run.review?.note" class="m-0 text-warn" data-testid="ade-review-note">
      {{ run.review.note }}
    </p>

    <section :aria-label="t('git.review.pane.comments.label')" class="flex flex-col gap-1.5">
      <p
        v-if="comments.length === 0"
        class="m-0 text-muted-foreground"
        data-testid="ade-review-no-comments"
      >
        {{ t('space.ade.reviewStep.noComments.hint') }}
      </p>
      <Card
        v-for="c in comments"
        :key="c.id"
        class="flex flex-col gap-1 p-2"
        data-testid="ade-review-comment"
        :data-comment-id="c.id"
        :data-state="c.state"
      >
        <div class="flex flex-wrap items-center gap-1.5">
          <Button
            variant="link"
            size="inline"
            class="font-data text-kira-sm text-muted-foreground"
            data-testid="ade-review-comment-path"
            @click="emit('openFile', c.path, c.origin)"
            >{{
              lineLabel(c)
            }}</Button
          >
          <AdeChip
            v-if="c.origin === 'working'"
            :label="t('space.ade.changes.uncommitted.label')"
            tone="amber"
            data-testid="ade-review-comment-uncommitted"
          />
          <AdeChip :label="kindLabel(c)" :tone="c.kind === 'question' ? 'purple' : 'blue'" />
          <AdeChip :label="stateLabel(c)" :tone="c.state === 'addressed' ? 'green' : 'amber'" />
          <Button
            variant="toolbar"
            size="kira"
            class="ml-auto"
            :pending="setState.isPending.value"
            data-testid="ade-review-comment-state"
            @click="
              act(() =>
                setState.mutateAsync({
                  id: c.id,
                  state: c.state === 'addressed' ? 'open' : 'addressed',
                }),
              )
            "
          >
            {{
              c.state === 'addressed'
                ? t('git.review.comments.reopen.label')
                : t('git.review.comments.markAddressed.label')
            }}
          </Button>
        </div>
        <p class="m-0 whitespace-pre-wrap break-words">{{ c.body }}</p>
        <p
          v-if="c.reply"
          class="m-0 whitespace-pre-wrap break-words border-l-2 border-border pl-2 text-muted-foreground"
          data-testid="ade-review-comment-reply"
        >
          {{ c.reply }}
        </p>
      </Card>
    </section>

    <section class="flex flex-col gap-1.5" data-testid="ade-review-send">
      <Label>{{ t('space.ade.workflows.stepInspector.reviewSendTo.label') }}</Label>
      <RadioGroup
        :model-value="route"
        class="flex flex-col gap-1"
        data-testid="ade-review-route"
        @update:model-value="
          (v) => {
            touched = true;
            route = v === 'new' ? 'new' : 'implementer';
          }
        "
      >
        <div class="flex items-center gap-2">
          <RadioGroupItem
            id="ade-review-route-implementer"
            value="implementer"
            data-testid="ade-review-route-implementer"
          />
          <Label for="ade-review-route-implementer">{{
            t('space.ade.reviewStep.route.implementer.label')
          }}</Label>
        </div>
        <NativeSelect
          v-if="route === 'implementer' && state && state.candidates.length > 0"
          variant="bordered"
          :model-value="targetRunId"
          data-testid="ade-review-target"
          @update:model-value="
            (v) => {
              touched = true;
              targetRunId = String(v);
            }
          "
        >
          <option v-for="cand in state.candidates" :key="cand.runId" :value="cand.runId">
            {{
              t('space.ade.reviewStep.candidate.label', { step: cand.step, attempt: cand.attempt })
            }}
          </option>
        </NativeSelect>
        <div class="flex items-center gap-2">
          <RadioGroupItem
            id="ade-review-route-new"
            value="new"
            data-testid="ade-review-route-new"
          />
          <Label for="ade-review-route-new">{{ t('space.ade.reviewStep.route.new.label') }}</Label>
        </div>
      </RadioGroup>
      <AdeTip :text="sendTip">
        <span class="inline-flex">
          <Button
            variant="dialog"
            size="kira"
            class="bg-agent text-tone-ink border-transparent"
            :disabled="sendTip !== ''"
            :pending="send.isPending.value"
            data-testid="ade-review-send-agent"
            @click="onSend"
          >
            {{ t('space.ade.reviewStep.send.label') }}
            <span class="text-kira-sm">{{
              t('space.ade.reviewStep.sendCount.label', { count: openCount })
            }}</span>
          </Button>
        </span>
      </AdeTip>
    </section>

    <section v-if="round" class="flex flex-col gap-1.5" data-testid="ade-review-round-block">
      <div class="flex flex-wrap items-center gap-2">
        <span v-if="round.commits !== null" class="text-kira-sm text-muted-foreground">{{
          t('space.ade.reviewStep.commits.label', { count: round.commits })
        }}</span>
        <Button
          v-if="live"
          variant="dialog"
          size="kira"
          data-testid="ade-review-stop"
          @click="act(() => stop.mutateAsync({ runId: round!.run.id }))"
        >
          {{ t('common.action.stop.label') }}
        </Button>
        <Button
          v-if="round.run.resumable"
          variant="dialog"
          size="kira"
          data-testid="ade-review-resume"
          @click="act(() => resume.mutateAsync({ runId: round!.run.id }))"
        >
          {{ t('common.action.resume.label') }}
        </Button>
        <Button
          variant="toolbar"
          size="kira"
          data-testid="ade-review-log-toggle"
          @click="showLog = !showLog"
        >
          {{ showLog ? t('space.common.hideLog.label') : t('space.common.showLog.label') }}
        </Button>
      </div>
      <AdeRunOutcome
        v-if="roundBad"
        :run="round.run"
        :title="t('space.ade.reviewStep.round.label', { n: round.n })"
      />
      <AdeRunLog v-if="showLog" kind="run" :id="round.run.id" max-height="180px" />
    </section>

    <FieldError v-if="error" data-testid="ade-review-error">{{ error }}</FieldError>

    <section v-if="state" class="flex flex-col gap-1.5" data-testid="ade-review-finish">
      <Textarea
        v-model="note"
        rows="2"
        maxlength="2000"
        :placeholder="t('space.ade.reviewStep.finish.note.placeholder')"
        data-testid="ade-review-finish-note"
      />
      <div class="flex flex-wrap gap-1.5">
        <Button
          v-for="r in state.results"
          :key="r.id"
          variant="dialog"
          size="kira"
          :class="TONE_SOLID_CLASS[r.ok ? 'green' : 'amber']"
          :disabled="live || !waiting"
          :pending="finish.isPending.value"
          :data-testid="`ade-review-finish-${r.id}`"
          @click="onFinish(r.id)"
        >
          {{ r.id }}
        </Button>
      </div>
    </section>

    <ConfirmDialogView
      :open="confirmResult !== null"
      :title="t('space.ade.reviewStep.finishOpen.title')"
      :message="t('space.ade.reviewStep.finishOpen.body', { count: openCount })"
      :confirm-label="t('space.ade.reviewStep.finishAnyway.label')"
      tone="primary"
      :testids="{
        root: 'ade-confirm-dialog',
        cancel: 'ade-confirm-no',
        confirm: 'ade-confirm-yes',
      }"
      @confirm="confirmFinish"
      @cancel="confirmResult = null"
    />
  </div>
</template>
