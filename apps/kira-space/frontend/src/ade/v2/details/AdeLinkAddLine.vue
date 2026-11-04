<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { errorText } from '@workbench/bridge/codedError';
import { ref, watch } from 'vue';
import AdeRow from '../panel/AdeRow.vue';
import AdeRowDetail from '../panel/AdeRowDetail.vue';
import { rowMeta } from '../panel/adeRow';
import type { LinkResult } from '../wire';

// The always-present last row of the links list. Type or paste a line and press Enter: the backend
// files it. Pasting into the empty line files at once; with text already typed the paste lands in the
// line, so a title can precede its link. Several pasted lines go in order.
const props = defineProps<{ add: (line: string) => Promise<LinkResult> }>();

const text = ref('');
const error = ref('');
const known = ref(false);

watch(text, (value) => {
  if (value !== '') known.value = false;
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
  if (e.isComposing) return;
  if (e.key === 'Enter') {
    e.preventDefault();
    run(text.value);
  } else if (e.key === 'Escape') {
    e.preventDefault();
    text.value = '';
    error.value = '';
    (e.target as HTMLElement).blur();
  }
}
</script>

<template>
  <li class="flex min-w-0 flex-col" data-testid="ade-details-add">
    <AdeRow label="">
      <template #label>
        <span class="flex w-full justify-center text-muted-foreground"
          ><CodiconIcon name="add" :size="13" /></span
        >
      </template>
      <Input
        v-model="text"
        :placeholder="t('space.ade.details.addLine.placeholder')"
        :aria-label="t('space.ade.details.addLine.aria')"
        appearance="inline"
        size="kira"
        class="min-w-0 flex-1"
        data-testid="ade-details-paste"
        @keydown="onKeydown"
        @paste="onPaste"
      />
    </AdeRow>
    <AdeRowDetail v-if="known" labelled>
      <div :class="rowMeta" data-testid="ade-details-paste-known">{{
        t('space.ade.details.addLine.known.body')
      }}</div>
    </AdeRowDetail>
    <AdeRowDetail v-if="error" labelled>
      <Alert variant="error" data-testid="ade-details-paste-error"
        ><AlertDescription>{{ error }}</AlertDescription></Alert
      >
    </AdeRowDetail>
  </li>
</template>
