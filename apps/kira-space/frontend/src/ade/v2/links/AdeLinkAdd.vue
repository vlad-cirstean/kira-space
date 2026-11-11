<script setup lang="ts">
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { errorText } from '@workbench/bridge/codedError';
import { ref, watch } from 'vue';
import type { LinkResult } from '../wire';

// The add popover behind the strip's `+`: type or paste a line and press Enter, the backend files it.
// Pasting into the empty input files at once; with text already typed the paste lands in the input, so a
// title can precede its link. Several pasted lines go in order. The popover stays open after an add, so
// several links go in a row; closing it drops the text.
const props = defineProps<{ add: (line: string) => Promise<LinkResult> }>();
const open = defineModel<boolean>('open', { default: false });

const text = ref('');
const error = ref('');
const known = ref(false);

watch(text, (value) => {
  if (value !== '') known.value = false;
});
watch(open, (isOpen) => {
  if (isOpen) return;
  text.value = '';
  error.value = '';
  known.value = false;
});

async function submit(raw: string): Promise<void> {
  const lines = raw
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter(Boolean);
  if (lines.length === 0) return;
  error.value = '';
  let allKnown = true;
  for (const [i, line] of lines.entries()) {
    try {
      const res = await props.add(line);
      if (res.filed !== 'known') allKnown = false;
    } catch (err) {
      text.value = lines.slice(i).join('\n');
      error.value = errorText(err);
      return;
    }
  }
  text.value = '';
  known.value = allKnown;
}

const submitting = useAsyncAction(submit);
function run(raw: string): void {
  if (!submitting.pending.value) void submitting.run(raw);
}

function onPaste(e: ClipboardEvent): void {
  if (text.value !== '') return;
  e.preventDefault();
  run(e.clipboardData?.getData('text') ?? '');
}

function onKeydown(e: KeyboardEvent): void {
  if (e.isComposing || e.key !== 'Enter') return;
  e.preventDefault();
  run(text.value);
}

// The strip files a paste that landed on a pill through the same path.
defineExpose({
  file(raw: string): void {
    open.value = true;
    run(raw);
  },
});
</script>

<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child><slot /></PopoverTrigger>
    <PopoverContent align="end" class="w-80 gap-1.5" data-testid="ade-links-add-popover">
      <Input
        v-model="text"
        :placeholder="t('space.ade.details.addLine.placeholder')"
        :aria-label="t('space.ade.details.addLine.aria')"
        size="kira"
        data-testid="ade-links-paste"
        @keydown="onKeydown"
        @paste="onPaste"
      />
      <FieldDescription v-if="known" data-testid="ade-links-paste-known">{{
        t('space.ade.details.addLine.known.body')
      }}</FieldDescription>
      <FieldError v-if="error" data-testid="ade-links-paste-error">{{ error }}</FieldError>
    </PopoverContent>
  </Popover>
</template>
