<script setup lang="ts">
// One review thread's body over Monaco, mounted into the overlay widget of a thread by
// reviewDecorations.ts via createApp/unmount, never routed through the workbench's own tab system
// (editor-surface furniture, not a view). 'view' renders an existing ReviewComment; 'compose' is
// the add-comment form; 'edit' reuses that form for an existing comment. Transport calls stay in
// reviewDecorations.ts: the actions below are async callbacks that reject on failure, and this
// component shows the error inline.
import type { ReviewComment, ReviewCommentKind, ReviewCommentState } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Textarea } from '@theme/components/ui/textarea';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import { useEventListener, useFocus, useResizeObserver, useTextareaAutosize } from '@vueuse/core';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, ref, type ShallowRef, useTemplateRef } from 'vue';
import { anchorParts } from './reviewAnchor';

const props = defineProps<{
  /** The mode the thread opens in. */
  mode: 'view' | 'compose';
  /** A live ref: a repaint after a save swaps in the stored comment without remounting. */
  comment?: Readonly<ShallowRef<ReviewComment | undefined>>;
  /** The review session's branch — reviewComments.ts:63-74's own anchorLabel needs it for the
   *  'removed'/'stale' message text; 'exact'/'projected' render no label (current lines, no
   *  caveat). */
  branch: string;
  onSubmit?: (body: string, kind: ReviewCommentKind) => Promise<void>;
  onEdit?: (body: string, kind: ReviewCommentKind) => Promise<void>;
  onSetState?: (state: ReviewCommentState) => Promise<void>;
  onDelete?: () => Promise<void>;
}>();

const emit = defineEmits<{
  close: [];
  cancel: [];
  /** Height of the thread's content including borders, for the host to size its zone. */
  resize: [height: number];
}>();

const current = computed(() => props.comment?.value);
const mode = ref<'view' | 'compose' | 'edit'>(props.mode);
const anchorLabel = computed(() => anchorParts(current.value, props.branch));

const draft = ref('');
const kind = ref<ReviewCommentKind>('question');
const KIND_ITEMS = computed(() => [
  {
    value: 'question',
    label: t('space.ade.needs.kind.question.label'),
    testid: 'review-thread-kind-question',
  },
  {
    value: 'change',
    label: t('git.review.comments.kind.change.label'),
    testid: 'review-thread-kind-change',
  },
]);
const nextState = computed<ReviewCommentState>(() =>
  current.value?.state === 'addressed' ? 'open' : 'addressed',
);

const root = useTemplateRef<HTMLElement>('root');
const content = useTemplateRef<HTMLElement>('content');
const input = useTemplateRef<{ $el: HTMLTextAreaElement }>('input');
const inputEl = computed(() => input.value?.$el);
useTextareaAutosize({ element: inputEl, input: draft });
useFocus(inputEl, { initialValue: true });

useResizeObserver(content, () => {
  const box = root.value;
  const inner = content.value;
  if (!box || !inner) return;
  emit('resize', inner.offsetHeight + box.offsetHeight - box.clientHeight);
});

// The editor's scrollbar listens for wheel on the whole editor and consumes it; keep a wheel that
// this box can still scroll away from it.
useEventListener(
  root,
  'wheel',
  (e: WheelEvent) => {
    const box = root.value;
    if (!box || box.scrollHeight <= box.clientHeight) return;
    const atEdge =
      e.deltaY > 0 ? box.scrollTop + box.clientHeight >= box.scrollHeight - 1 : box.scrollTop <= 0;
    if (!atEdge) e.stopPropagation();
  },
  { passive: true },
);

const { run, pending, error } = useAsyncAction(async (action: () => Promise<void> | undefined) => {
  await action();
});

async function submit(): Promise<void> {
  const body = draft.value.trim();
  if (body === '' || pending.value) return;
  if (mode.value === 'edit') {
    if (await run(() => props.onEdit?.(body, kind.value))) mode.value = 'view';
  } else {
    await run(() => props.onSubmit?.(body, kind.value));
  }
}

function startEdit(): void {
  const comment = current.value;
  if (!comment) return;
  draft.value = comment.body;
  kind.value = comment.kind;
  error.value = null;
  mode.value = 'edit';
}

function cancelEdit(): void {
  error.value = null;
  mode.value = 'view';
}

function act(action: () => Promise<void> | undefined): void {
  if (!pending.value) void run(action);
}

useShortcut(root, ['form.submit', 'form.cancel'], (id) => {
  if (id === 'form.submit') void submit();
  else if (mode.value === 'edit') cancelEdit();
  else if (mode.value === 'compose') emit('cancel');
  else emit('close');
});
</script>

<template>
  <!-- Monaco's view-lines layer otherwise wins text selection inside this zone too. -->
  <div
    ref="root"
    class="box-border h-full w-full overflow-auto bg-elevated border-t border-b border-border-strong text-fg text-kira-md select-text"
    data-testid="review-thread"
  >
    <div ref="content" class="flex flex-col gap-1.5 py-2 px-3">
      <template v-if="mode === 'view' && current">
        <div v-if="anchorLabel" class="text-warn italic"><VarText :parts="anchorLabel" /></div>
        <div class="flex items-center gap-1.5 text-kira-sm text-muted-foreground">
          <span data-testid="review-thread-kind-chip">{{
            current.kind === 'question'
              ? t('space.ade.needs.kind.question.label')
              : t('git.review.comments.kind.change.label')
          }}</span>
          <span data-testid="review-thread-state-chip">{{
            current.state === 'addressed'
              ? t('git.review.comments.state.addressed.label')
              : t('common.action.open.label')
          }}</span>
        </div>
        <div class="whitespace-pre-wrap break-words" data-testid="review-thread-body">
          {{ current.body }}
        </div>
        <div
          v-if="current.reply"
          class="whitespace-pre-wrap break-words border-l-2 border-border-strong pl-2 text-muted-foreground"
          data-testid="review-thread-reply"
        >
          {{ current.reply }}
        </div>
        <Alert v-if="error" variant="error" data-testid="review-thread-error"
          ><AlertDescription>{{ error }}</AlertDescription></Alert
        >
        <div class="flex flex-wrap justify-end gap-1.5">
          <Button
            variant="toolbar"
            size="kira"
            :pending="pending"
            data-testid="review-thread-state"
            @click="act(() => props.onSetState?.(nextState))"
          >
            {{
              current.state === 'addressed'
                ? t('git.review.comments.reopen.label')
                : t('git.review.comments.markAddressed.label')
            }}
          </Button>
          <Button variant="toolbar" size="kira" data-testid="review-thread-edit" @click="startEdit">
            <CodiconIcon name="pencil" :size="13" />
            {{ t('common.action.edit.label') }}
          </Button>
          <Button variant="toolbar" size="kira" @click="emit('close')">
            <CodiconIcon name="close" :size="13" />
            {{ t('common.action.close.label') }}
          </Button>
          <Button
            variant="danger"
            size="kira"
            :pending="pending"
            @click="act(() => props.onDelete?.())"
          >
            <CodiconIcon name="trash" :size="13" />
            {{ t('common.action.delete.label') }}
          </Button>
        </div>
      </template>
      <template v-else>
        <SecondaryTabs
          v-model="kind"
          variant="segmented"
          :items="KIND_ITEMS"
          data-testid="review-thread-kind"
        />
        <Textarea
          ref="input"
          v-model="draft"
          class="resize-none"
          :placeholder="t('space.repo.view.comment.placeholder')"
          rows="3"
          data-testid="review-thread-input"
        />
        <Alert v-if="error" variant="error" data-testid="review-thread-error"
          ><AlertDescription>{{ error }}</AlertDescription></Alert
        >
        <div class="flex flex-wrap justify-end gap-1.5">
          <Button
            variant="toolbar"
            size="kira"
            data-testid="review-thread-cancel"
            @click="mode === 'edit' ? cancelEdit() : emit('cancel')"
            >{{
              t('common.action.cancel.label')
            }}</Button
          >
          <Button
            variant="toolbar-primary"
            size="kira"
            :pending="pending"
            :disabled="draft.trim() === ''"
            :data-testid="mode === 'edit' ? 'review-thread-save' : 'review-thread-send'"
            @click="submit"
          >
            <CodiconIcon :name="mode === 'edit' ? 'save' : 'comment'" :size="13" />
            {{ mode === 'edit' ? t('common.action.save.label') : t('common.comment.label') }}
          </Button>
        </div>
      </template>
    </div>
  </div>
</template>
