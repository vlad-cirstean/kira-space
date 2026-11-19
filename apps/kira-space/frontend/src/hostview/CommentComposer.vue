<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card, CardContent } from '@theme/components/ui/card';
import { Textarea } from '@theme/components/ui/textarea';
import { formatBytes } from '@theme/format';
import { t } from '@theme/i18n';
import { useSessionStorage } from '@vueuse/core';
import { errorText } from '@workbench/bridge/codedError';
import { windowKey } from '@workbench/bridge/rpc';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { computed, ref } from 'vue';
import { gapText } from '../ade/v2/artifacts/gapText';
import MarkdownBody from './MarkdownBody.vue';
import { PostRejected, usePostComment } from './queries';
import type { HostViewCompose } from './wire';

// One comment composer: Markdown text and Post, no attachments. Text stays exactly as typed
// (Markdown source, not rich text). A failed post keeps the text.
const props = defineProps<{
  compose: HostViewCompose;
  kind: 'changeRequest' | 'issue';
  /** The query the optimistic comment is written to. */
  queryKey: readonly unknown[];
  /** Tracker id, for the reason words of an issue failure. */
  provider: string;
  replyTo?: string;
  compact?: boolean;
}>();
const emit = defineEmits<{ posted: []; cancel: [] }>();

const body = useSessionStorage(
  () => `kira.hostview.draft:${windowKey}:${props.kind}:${props.replyTo ?? ''}`,
  '',
);
const mode = ref('write');
const error = ref('');

const post = usePostComment({ windowKey, kind: props.kind, queryKey: () => props.queryKey });

const bytes = computed(() => new TextEncoder().encode(body.value).length);
const overCap = computed(() => bytes.value > props.compose.maxBodyBytes);
const nearCap = computed(() => bytes.value > props.compose.maxBodyBytes * 0.8);
const canPost = computed(() => !post.isPending.value && !overCap.value && body.value.trim() !== '');

function failureText(err: unknown): string {
  if (!(err instanceof PostRejected)) return errorText(err);
  if (err.kind === 'gone') return t('space.hostView.gone.body');
  const text =
    props.kind === 'issue'
      ? gapText({ source: props.provider, repo: '', reason: err.reason || 'failed' })
      : err.reason;
  const timedOut =
    props.kind === 'issue' ? err.reason === 'timeout' : /may have been recorded/.test(err.reason);
  return timedOut ? `${text} ${t('space.hostView.compose.mayHavePosted.hint')}` : text;
}

async function submit(): Promise<void> {
  if (!canPost.value) return;
  error.value = '';
  try {
    await post.mutateAsync({ body: body.value, replyTo: props.replyTo ?? '' });
    body.value = '';
    mode.value = 'write';
    emit('posted');
  } catch (err) {
    error.value = failureText(err);
  }
}

const onSubmitKey = shortcutKeydown(['form.submit'], () => void submit());
const modes = computed(() => [
  { value: 'write', label: t('common.action.edit.label'), testid: 'comment-mode-write' },
  { value: 'preview', label: t('common.tab.preview.label'), testid: 'comment-mode-preview' },
]);
</script>

<template>
  <Card
    variant="section"
    class="relative"
    :data-compact="compact || undefined"
    data-testid="comment-composer"
  >
    <CardContent class="flex flex-col gap-2">
      <div class="flex items-center gap-2">
        <SecondaryTabs
          v-model="mode"
          :items="modes"
          variant="segmented"
          :aria-label="t('space.hostView.compose.mode.aria')"
        />
      </div>
      <Textarea
        v-if="mode === 'write'"
        v-model="body"
        :aria-label="t('common.comment.label')"
        :placeholder="t('space.hostView.compose.body.placeholder')"
        data-testid="comment-body"
        @keydown="onSubmitKey"
      />
      <Card v-else class="min-h-16" data-testid="comment-preview">
        <MarkdownBody v-if="body.trim()" :source="body" />
        <InlineState kind="empty" v-else>{{
          t('space.hostView.compose.preview.empty')
        }}</InlineState>
      </Card>
      <Alert v-if="error" variant="error" data-testid="comment-error">
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
      <div class="flex items-center gap-2">
        <span
          v-if="nearCap"
          class="text-kira-sm"
          :class="overCap ? 'text-error' : 'text-muted-foreground'"
          data-testid="comment-cap"
          >{{
            t('common.ofTotal.label', {
              value: formatBytes(bytes),
              total: formatBytes(compose.maxBodyBytes),
            })
          }}</span
        >
        <div class="ml-auto flex items-center gap-2">
          <Button
            v-if="replyTo"
            variant="dialog"
            size="kira"
            data-testid="comment-cancel"
            @click="emit('cancel')"
          >
            {{ t('common.action.cancel.label') }}
          </Button>
          <Button
            variant="dialog-primary"
            size="kira"
            :pending="post.isPending.value"
            :disabled="!canPost"
            data-testid="comment-post"
            @click="submit"
          >
            {{ t('common.comment.label') }}
          </Button>
        </div>
      </div>
    </CardContent>
  </Card>
</template>
