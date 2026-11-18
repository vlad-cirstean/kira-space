<script setup lang="ts">
import type { RoutedPrompt } from '@shared/domain/prompts';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { Input } from '@theme/components/ui/input';
import { Kbd } from '@theme/components/ui/kbd';
import { t } from '@theme/i18n';
import { useFocusOnMount } from '@workbench/behaviour/focus';
import SecretInput from '@workbench/components/SecretInput.vue';
import { formatShortcut } from '@workbench/shortcuts/keys';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { type ComponentPublicInstance, computed, ref, useId, useTemplateRef } from 'vue';
import { useGitCredentialStore } from '../state/gitCredential';

// git's own askpass prompt, mounted by PromptHost for the routed `git-credential` entry whose ref
// is a relay request id. Shift+Escape and the frame's close hide it in this window
// (the op keeps waiting); Cancel answers null (the op fails). The typed value lives only in
// `value`, cleared in the same statement that answers, and is never logged or persisted.
const props = defineProps<{ entry: RoutedPrompt; more: number }>();
const emit = defineEmits<{ hide: [] }>();
const store = useGitCredentialStore();
const prompt = computed(() => store.prompts.find((p) => p.requestId === props.entry.ref) ?? null);

const value = ref('');
const promptId = useId();
const secretField = useTemplateRef<{ input?: HTMLInputElement }>('secretField');
const plainField = useTemplateRef<ComponentPublicInstance>('plainField');

useFocusOnMount(computed(() => secretField.value?.input ?? plainField.value));

function onSubmit(): void {
  if (!prompt.value) return;
  const secret = value.value;
  value.value = '';
  void store.answer(prompt.value.requestId, secret);
}

function onCancel(): void {
  value.value = '';
  if (prompt.value) void store.answer(prompt.value.requestId, null);
}

// Two-key chords only: Enter in the field types, never submits. Mod+Backspace deletes text in the
// field, so it cancels only from outside it.
const submitKey = shortcutKeydown(['form.submit'], onSubmit);
const cancelKey = shortcutKeydown(['prompt.deny'], onCancel, { ignoreTextFields: true });
function onChord(event: KeyboardEvent): void {
  submitKey(event);
  cancelKey(event);
}
</script>

<template>
  <Dialog v-if="prompt" :open="true" @update:open="(v) => !v && emit('hide')">
    <DialogContent
      size="sm"
      prompt
      :dirty="value !== ''"
      data-testid="git-credential-dialog"
      @keydown="onChord"
    >
      <DialogHeader close-testid="git-credential-dialog-close">
        <DialogTitle>{{ t('space.gitCredential.dialog.title') }}</DialogTitle>
        <DialogDescription data-testid="git-credential-repo">
          {{ t('common.dotPair.label', { first: prompt.source, second: prompt.repoLabel }) }}
        </DialogDescription>
      </DialogHeader>

      <DialogBody>
        <!-- git's own text, rendered verbatim — never reformatted, never parsed. -->
        <p
          :id="promptId"
          class="m-0 font-data whitespace-pre-wrap"
          data-testid="git-credential-prompt"
        >
          {{ prompt.prompt }}
        </p>
        <SecretInput
          v-if="prompt.masked"
          ref="secretField"
          v-model="value"
          :aria-labelledby="promptId"
          data-testid="git-credential-input"
          data-dialog-initial
        />
        <Input
          :aria-labelledby="promptId"
          v-else
          ref="plainField"
          v-model="value"
          class="w-full font-data"
          data-testid="git-credential-input"
          data-dialog-initial
        />
      </DialogBody>

      <DialogFooter>
        <template v-if="more > 0" #start>
          <span class="text-kira-sm text-muted-foreground" data-testid="git-credential-more">
            {{ t('common.moreWaiting.status', { count: more }) }}
          </span>
        </template>
        <Button
          variant="dialog"
          size="kira-lg"
          :aria-keyshortcuts="formatShortcut('prompt.deny')"
          data-testid="git-credential-cancel"
          @click="onCancel"
        >
          {{ t('common.action.cancel.label') }}
          <Kbd>{{ formatShortcut('prompt.deny') }}</Kbd>
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :aria-keyshortcuts="formatShortcut('form.submit')"
          data-testid="git-credential-submit"
          @click="onSubmit"
        >
          {{ t('common.continue.label') }}
          <Kbd>{{ formatShortcut('form.submit') }}</Kbd>
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
