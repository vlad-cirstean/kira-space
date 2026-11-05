<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { storeToRefs } from 'pinia';
import { computed } from 'vue';
import { formatCurrency } from '../../behaviour/format';
import { errorText, toCodedError } from '../../bridge/codedError';
import PanelBar from '../../components/PanelBar.vue';
import { useMemoryUiStore } from '../store';
import DistillProgress from './DistillProgress.vue';
import DistillResult from './DistillResult.vue';
import {
  useChooseDistillFile,
  useDistill,
  useDistillLimits,
  useDistillProgress,
  useDistillRunning,
} from './distillQueries';
import { useDistillUiStore } from './distillStore';
import { countChars, estimateTokens } from './size';

// P285 Part 4: the main area's distill view. Same pipeline as `ingest_files` with distill true;
// read-only until the user stores or imports the result.
const memoryUi = useMemoryUiStore();
const ui = useDistillUiStore();
const { source, text, mode, path, origin, progress, cancelled } = storeToRefs(ui);
const limits = useDistillLimits();
const distill = useDistill();
const choose = useChooseDistillFile();
const inFlight = useDistillRunning();
useDistillProgress();

const SOURCES = computed(() => [
  { value: 'text', label: t('common.field.text.label'), testid: 'distill-source-text' },
  { value: 'file', label: t('common.field.file.label'), testid: 'distill-source-file' },
]);
const MODES = computed(() => [
  { value: 'facts', label: t('memory.distill.mode.facts.label'), testid: 'distill-mode-facts' },
  {
    value: 'rewrite',
    label: t('memory.distill.mode.rewrite.label'),
    testid: 'distill-mode-rewrite',
  },
]);
const rewriting = computed(() => source.value === 'text' && mode.value === 'rewrite');
const chars = computed(() => countChars(text.value));
const maxChars = computed(
  () =>
    (rewriting.value ? limits.data.value?.rewriteMaxChars : limits.data.value?.maxChars) ??
    Number.POSITIVE_INFINITY,
);
const overLimit = computed(() => source.value === 'text' && chars.value > maxChars.value);
const canRun = computed(
  () =>
    !inFlight.value &&
    !overLimit.value &&
    (source.value === 'text' ? text.value.trim() !== '' : path.value !== ''),
);
const error = computed(() => (distill.isError.value ? distill.error.value : null));
const wasCancelled = computed(
  () =>
    error.value !== null && (cancelled.value || toCodedError(error.value).code === 'E_CANCELLED'),
);
const originLabel = computed(() =>
  origin.value
    ? t('memory.distill.origin.label', {
        path: origin.value.path,
        start: origin.value.start,
        end: origin.value.end,
      })
    : '',
);

function onEdit(value: string | number): void {
  text.value = String(value);
  origin.value = null;
}

async function pickFile(): Promise<void> {
  const picked = await choose.mutateAsync().catch(() => null);
  if (picked) path.value = picked;
}

async function run(): Promise<void> {
  const signal = ui.begin();
  const fromText = source.value === 'text';
  const from = origin.value
    ? `${origin.value.path}:${origin.value.start}-${origin.value.end}`
    : fromText
      ? ''
      : path.value;
  try {
    await distill.mutateAsync({
      args: {
        requestId: ui.requestId,
        ...(fromText ? { text: text.value, mode: mode.value } : { path: path.value }),
      },
      signal,
      source: from,
    });
  } catch {
    // Shown through distill.error.
  } finally {
    ui.end();
  }
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="distill-view">
    <PanelBar class="flex-row flex-wrap items-center gap-1.5">
      <Button
        variant="toolbar"
        size="kira"
        data-testid="distill-back"
        @click="memoryUi.view = 'memory'"
        >{{
          t('memory.import.back.label')
        }}</Button
      >
      <SecondaryTabs v-model="source" variant="segmented" :items="SOURCES" />
      <SecondaryTabs v-if="source === 'text'" v-model="mode" variant="segmented" :items="MODES" />
    </PanelBar>
    <div class="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
      <FieldDescription data-testid="distill-hint">{{
        t(rewriting ? 'memory.distill.rewrite.hint' : 'memory.distill.hint')
      }}</FieldDescription>
      <template v-if="source === 'text'">
        <Badge v-if="origin" class="self-start" data-testid="distill-origin">{{
          originLabel
        }}</Badge>
        <Textarea
          class="min-h-40 font-data"
          :model-value="text"
          :disabled="inFlight"
          :placeholder="t('memory.distill.input.placeholder')"
          data-testid="distill-input"
          @update:model-value="onEdit"
        />
        <span
          class="text-kira-sm"
          :class="overLimit ? 'text-error' : 'text-muted-foreground'"
          data-testid="distill-size"
          >{{ t('memory.distill.size.status', { chars, tokens: estimateTokens(text) }) }}
          <template v-if="overLimit">
            · {{ t('memory.distill.overLimit.error', { max: maxChars }) }}</template
          ></span
        >
      </template>
      <div v-else class="flex min-w-0 items-center gap-2">
        <Button
          variant="dialog"
          size="kira"
          :pending="choose.isPending.value"
          :disabled="inFlight"
          data-testid="distill-choose"
          @click="pickFile"
          >{{
            t('memory.distill.file.choose.label')
          }}</Button
        >
        <span class="min-w-0 flex-1 text-kira-md" data-testid="distill-path">
          <TruncatedText v-if="path" :text="path" split="path" />
          <span v-else class="text-muted-foreground">{{ t('memory.distill.file.none.hint') }}</span>
        </span>
      </div>
      <div class="flex items-center gap-2">
        <Button
          v-if="!inFlight"
          variant="dialog-primary"
          size="kira"
          :disabled="!canRun"
          data-testid="distill-run"
          @click="run"
          >{{
            t('memory.distill.run.label')
          }}</Button
        >
        <DistillProgress v-else />
      </div>
      <Alert v-if="error && !wasCancelled" variant="error" data-testid="distill-error">
        <AlertDescription>{{ errorText(error) }}</AlertDescription>
      </Alert>
      <FieldDescription v-else-if="wasCancelled" data-testid="distill-cancelled">{{
        t('memory.distill.cancelled.status', { cost: formatCurrency(progress?.costUsd ?? 0) })
      }}</FieldDescription>
      <DistillResult v-if="!inFlight" />
    </div>
  </div>
</template>
