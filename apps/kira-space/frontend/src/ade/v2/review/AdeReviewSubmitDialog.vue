<script setup lang="ts">
import { shortSha } from '@kira/git-core';
import type { ChangeRequestReviewEvent, ChangeRequestReviewState } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription } from '@theme/components/ui/field';
import { Textarea } from '@theme/components/ui/textarea';
import { hostWords, type MessageKey, t } from '@theme/i18n';
import { useTaskAction } from '@workbench/behaviour/asyncAction';
import { errorText } from '@workbench/bridge/codedError';
import Identifier from '@workbench/components/Identifier.vue';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { computed, onMounted, ref } from 'vue';
import type { ReviewWindowTarget } from '../wire';
import { headStale, reviewGate, useChangeRequestReview } from './useChangeRequestReview';

type Mode = ChangeRequestReviewEvent | 'conversation';
type OkState = Extract<ChangeRequestReviewState, { kind: 'ok' }>;

const props = defineProps<{ mode: Mode; target: ReviewWindowTarget; review: OkState }>();
const emit = defineEmits<{ done: [kind: 'review' | 'comment']; close: [] }>();

const { state, submitReview, comment, openChangeRequest } = useChangeRequestReview(
  () => props.target,
);
// The refetch on open gives the head the review applies to. Submit waits for it, so a stale
// head or a PR merged meanwhile is never posted against.
const live = computed<OkState>(() =>
  state.data.value?.kind === 'ok' ? state.data.value : props.review,
);
const refreshError = computed(() => {
  if (state.isFetching.value) return '';
  if (state.isError.value) return errorText(state.error.value);
  const d = state.data.value;
  return d?.kind === 'unavailable' ? (d.status.reason ?? '') : '';
});
const gate = computed(() =>
  props.mode === 'conversation'
    ? { enabled: true, reason: '' }
    : reviewGate(live.value, props.mode),
);
onMounted(() => void state.refetch());

const TITLE: Record<Mode, MessageKey> = {
  approve: 'space.ade.review.actions.dialogApprove.title',
  comment: 'space.ade.review.actions.dialogComment.title',
  requestChanges: 'space.ade.review.actions.dialogRequestChanges.title',
  conversation: 'space.ade.review.actions.dialogConversation.title',
};
const SUBMIT: Record<Mode, MessageKey> = {
  approve: 'common.action.approve.label',
  comment: 'space.ade.review.actions.submitReview.label',
  requestChanges: 'space.ade.review.actions.requestChanges.label',
  conversation: 'common.comment.label',
};

const short = computed(() => hostWords(live.value.host).changeRequestShort);
const body = ref('');
const surface = useTaskAction();
const error = surface.error;
const required = computed(() => props.mode !== 'approve');
const posting = computed(() => submitReview.isPending.value || comment.isPending.value);
const refreshing = computed(() => state.isFetching.value);
const pending = computed(() => posting.value || refreshing.value);
const canSubmit = computed(
  () =>
    !pending.value &&
    !refreshError.value &&
    gate.value.enabled &&
    (!required.value || body.value.trim() !== ''),
);
const headDiffers = computed(() => props.mode !== 'conversation' && headStale(live.value));

async function submit(): Promise<void> {
  if (!canSubmit.value) return;
  await surface.run(async () => {
    const res =
      props.mode === 'conversation'
        ? await comment.mutateAsync({ number: live.value.number, body: body.value })
        : await submitReview.mutateAsync({
            number: live.value.number,
            event: props.mode,
            body: body.value,
            headSha: live.value.headSha,
          });
    if (res.kind === 'ok') {
      emit('done', props.mode === 'conversation' ? 'comment' : 'review');
      emit('close');
    } else if (res.kind === 'unavailable') {
      throw new Error(res.status.reason ?? '');
    } else {
      throw new Error(t('space.ade.review.actions.errorDisabled.body'));
    }
  });
}

const onSubmitKey = shortcutKeydown(['form.submit'], () => void submit());
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent
      size="md"
      resizable
      :dirty="body.trim() !== ''"
      data-testid="ade-review-submit-dialog"
    >
      <DialogHeader>
        <DialogTitle>{{ t(TITLE[mode], { short, number: live.number }) }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <Alert v-if="headDiffers" variant="warn" data-testid="ade-review-submit-head">
          <AlertDescription>
            <div class="flex flex-col gap-1">
              <span>{{ t('space.ade.review.actions.headDiffers.hint', { short }) }}</span>
              <span class="flex items-center gap-1.5 text-kira-sm">
                <Identifier :value="live.localHeadSha" :short="shortSha" />
                <CodiconIcon name="arrow-right" :size="12" />
                <Identifier :value="live.headSha" :short="shortSha" />
              </span>
            </div>
          </AlertDescription>
        </Alert>
        <Textarea
          v-model="body"
          rows="6"
          class="min-h-24 flex-auto resize-none field-sizing-fixed"
          data-dialog-initial
          :aria-label="t('common.message.label')"
          :placeholder="t('space.ade.review.actions.dialogBody.placeholder')"
          data-testid="ade-review-submit-body"
          @keydown="onSubmitKey"
        />
        <FieldDescription v-if="!gate.enabled" data-testid="ade-review-submit-gate">{{
          gate.reason
        }}</FieldDescription>
        <FieldDescription v-else>{{
          required
            ? t('space.ade.review.actions.required.hint')
            : t('space.ade.review.actions.optional.hint')
        }}</FieldDescription>
        <Alert v-if="refreshError" variant="error" data-testid="ade-review-submit-refresh-error">
          <AlertDescription>{{
            t('space.ade.review.actions.stateFailed.hint', { reason: refreshError })
          }}</AlertDescription>
        </Alert>
        <Alert v-if="error" variant="error" data-testid="ade-review-submit-error">
          <AlertDescription>
            <div class="flex items-start gap-3">
              <span class="min-w-0 flex-1">{{ error }}</span>
              <Button
                variant="toolbar"
                size="kira"
                class="shrink-0"
                data-testid="ade-review-submit-open"
                @click="openChangeRequest(live.number)"
                ><CodiconIcon name="go-to-file" :size="13" />
                {{ t('space.ade.review.actions.open.label', { short }) }}
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      </DialogBody>
      <DialogFooter>
        <DialogClose as-child>
          <Button variant="dialog" size="kira-lg">{{ t('common.action.cancel.label') }}</Button>
        </DialogClose>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :pending="pending"
          :disabled="!canSubmit"
          data-testid="ade-review-submit"
          @click="submit"
        >
          {{ t(SUBMIT[mode]) }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
