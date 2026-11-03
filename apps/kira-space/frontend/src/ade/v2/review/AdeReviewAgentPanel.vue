<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { TooltipDisabledTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { useAgentInfo } from '@workbench/agent/agentInfo';
import { useCopy } from '@workbench/behaviour/clipboard';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref } from 'vue';
import { notifyReviewRepaint } from '../../../repo/git/reviewRepaint';
import { useTerminalsStore } from '../../../state/terminals';
import { useAgentSessionsStore } from '../../state';
import AdeTip from '../AdeTip.vue';
import { activityKind, withTuiActivity } from '../activity';
import { blockingSetups } from '../board/setupGate';
import { useLaunchOpener } from '../dialog/useLaunchOpener';
import AdeSetupProgress from '../panel/AdeSetupProgress.vue';
import {
  useBoard,
  useFocusSession,
  useForwardReviewComments,
  useLaunchReviewAgent,
  useReviewAgent,
  useReviewForwardText,
} from '../queries';
import AdeTuiPane from '../sessions/AdeTuiPane.vue';
import { type SessionLookup, sessionView } from '../sessions/sessionView';
import { isPreparing, useSetupWait } from '../state/useSetupWait';
import type { ReviewWindowTarget } from '../wire';
import { useReviewComments } from './useReviewComments';

// The review agent of this window's task: its terminal when this window runs it, a pointer to the
// window that does, or a Start button. One process per task, never two.
const props = defineProps<{ target: ReviewWindowTarget; repoLabel: string }>();

const terminals = useTerminalsStore();
const launchOpener = useLaunchOpener();
const agentStore = useAgentSessionsStore();
const agent = useReviewAgent(() => props.target.taskId);
const launchM = useLaunchReviewAgent();
const focusM = useFocusSession();
const forwardM = useForwardReviewComments();
const textM = useReviewForwardText();
const { copy: copyText, copied } = useCopy();
const comments = useReviewComments(() => props.target);

const board = useBoard();
// The review window's worktree may still be preparing: Start waits for it instead of failing.
const setups = computed(() =>
  blockingSetups(board.data.value, props.target.taskId, [props.target.branchId]),
);
const setupWait = useSetupWait(() => start());
const waitingSetup = setupWait.waiting;

const note = ref('');
const error = ref('');
const missing = ref(false);

const session = computed(() => {
  const s = agent.data.value?.session;
  return s ? (withTuiActivity([s], agentStore.activity)[0] ?? null) : null;
});
const running = computed(() => session.value?.state === 'running');
const here = computed(
  () => running.value && terminals.terminalSession(session.value?.terminalId ?? '') !== undefined,
);

const openCount = computed(
  () => (comments.query.data.value ?? []).filter((c) => c.state === 'open').length,
);
const waitingOnInput = computed(
  () => session.value !== null && activityKind(session.value) === 'input',
);
const forwardTip = computed(() => {
  if (waitingOnInput.value) return t('space.ade.review.forward.blocked.tooltip');
  if (openCount.value === 0) return t('space.ade.review.forward.none.tooltip');
  return '';
});
const copyOff = computed(() => openCount.value === 0 || textM.isPending.value);
const forwardOff = computed(() => forwardTip.value !== '' || forwardM.isPending.value);

async function forward(): Promise<void> {
  error.value = '';
  try {
    await forwardM.mutateAsync({ branchId: props.target.branchId });
    notifyReviewRepaint({
      kind: 'comments',
      repoId: props.target.gitRepoId,
      branch: props.target.branch,
    });
  } catch (err) {
    error.value = errorText(err);
  } finally {
    await comments.refresh();
  }
}

async function copyComments(): Promise<void> {
  error.value = '';
  try {
    const { text } = await textM.mutateAsync({ branchId: props.target.branchId });
    await copyText(text);
  } catch (err) {
    error.value = errorText(err);
  }
}

const { info: agentInfo } = useAgentInfo();
const lookup = computed<SessionLookup>(() => ({
  task: () => undefined,
  branch: () => undefined,
  repoLabel: () => '',
  workflow: () => undefined,
  headlessLabel: agentInfo.value.headlessLabel,
}));
const view = computed(() => (session.value ? sessionView(session.value, lookup.value) : null));

async function start(): Promise<void> {
  error.value = '';
  note.value = '';
  try {
    const result = await launchM.mutateAsync({ taskId: props.target.taskId });
    setupWait.stop();
    note.value = result.note;
    await launchOpener.open(result.launch);
  } catch (err) {
    if (isPreparing(err)) {
      setupWait.start();
    } else {
      setupWait.stop();
      error.value = errorText(err);
    }
  } finally {
    await agent.refetch();
  }
}

async function focus(): Promise<void> {
  if (!session.value) return;
  missing.value = false;
  const shown = await focusM
    .mutateAsync({ sessionId: session.value.id, taskId: props.target.taskId })
    .catch(() => false);
  if (!shown) missing.value = true;
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="ade-review-agent">
    <Alert v-if="note" variant="info" strip data-testid="ade-review-note">
      <AlertDescription>{{ note }}</AlertDescription>
    </Alert>
    <template v-if="session && running && view">
      <div
        class="flex flex-none items-center gap-2 border-b border-border px-2 py-1.5"
        data-testid="ade-review-forward-bar"
      >
        <AdeTip :text="forwardTip">
          <TooltipDisabledTrigger :disabled="forwardOff">
            <Button
              variant="dialog"
              size="kira"
              :pending="forwardM.isPending.value"
              :disabled="forwardOff"
              data-testid="ade-review-forward"
              @click="forward"
            >
              <CodiconIcon name="comment-discussion" :size="13" />
              {{ t('space.ade.review.forward.label', { count: openCount }) }}
            </Button>
          </TooltipDisabledTrigger>
        </AdeTip>
        <AdeTip
          :text="
            openCount === 0
              ? t('space.ade.review.copy.none.tooltip')
              : t('space.ade.review.copy.tooltip')
          "
        >
          <TooltipDisabledTrigger :disabled="copyOff">
            <Button
              variant="toolbar"
              size="kira"
              :disabled="copyOff"
              data-testid="ade-review-copy"
              @click="copyComments"
            >
              <CodiconIcon :name="copied ? 'check' : 'copy'" :size="13" />
              {{ copied ? t('common.copy.copied.announce') : t('common.action.copy.label') }}
            </Button>
          </TooltipDisabledTrigger>
        </AdeTip>
      </div>
      <AdeTuiPane v-if="here" :view="view" class="min-h-0 flex-1" />
      <div
        v-else
        class="flex flex-col items-start gap-2 p-3 text-kira-md"
        data-testid="ade-review-elsewhere"
      >
        <p class="m-0">{{ t('space.ade.review.agent.elsewhere.hint') }}</p>
        <Button variant="dialog" size="kira" data-testid="ade-review-focus" @click="focus">{{
          t('space.ade.review.agent.focus.label')
        }}</Button>
        <FieldDescription v-if="missing">{{
          t('space.ade.review.agent.gone.hint')
        }}</FieldDescription>
      </div>
    </template>
    <div
      v-else
      class="flex flex-col items-start gap-2 p-3 text-kira-md"
      data-testid="ade-review-start-box"
    >
      <FieldDescription>{{ t('space.ade.review.agent.intro.hint') }}</FieldDescription>
      <AdeSetupProgress
        v-for="b in setups"
        :key="b.id"
        class="w-full"
        :branch="b"
        :repo="repoLabel"
        data-testid="ade-review-setup"
      />
      <Button
        variant="dialog"
        size="kira"
        :pending="launchM.isPending.value"
        :disabled="waitingSetup"
        data-testid="ade-review-start"
        @click="start"
      >
        {{
          waitingSetup
            ? t('space.ade.agentDialog.startsWhenReady.loading')
            : session
              ? t('space.ade.review.agent.resume.label')
              : t('space.ade.review.agent.start.label')
        }}
      </Button>
    </div>
    <div v-if="error" class="px-3 pb-2">
      <Alert variant="error" data-testid="ade-review-start-error"
        ><AlertDescription>{{ error }}</AlertDescription></Alert
      >
    </div>
  </div>
</template>
