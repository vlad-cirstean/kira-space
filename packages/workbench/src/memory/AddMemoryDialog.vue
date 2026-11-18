<script setup lang="ts">
import type { MemoryClarification, MemoryScope, MemoryStoreResult } from '@shared/domain/memory';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { computed, reactive, ref } from 'vue';
import { errorText } from '../bridge/codedError';
import { useStoreMemory } from './queries';
import ScopeField from './ScopeField.vue';
import { useMemoryUiStore } from './store';

// P214: free-text Add memory through the MCP store_memory gate. A challenge stores nothing; answer or edit, resubmit.
// P285 Part 4: with `lines` (distilled output) it stores one fact per line instead of the text field.
const props = defineProps<{ lines?: string[]; source?: string }>();
const emit = defineEmits<{ close: [] }>();
const ui = useMemoryUiStore();
const store = useStoreMemory();

const MAX_TEXT = 1000;
const MANUAL_REASON = 'Stated by the user, added manually in the Memory module.';
const distilledReason = (source: string): string =>
  `Distilled by the user from ${source || 'pasted text'} in the Memory module.`;
const text = ref('');
const scope = ref<MemoryScope>('personal');
const project = ref('');
const SCOPES: MemoryScope[] = ['personal', 'project', 'company'];
const result = ref<MemoryStoreResult | null>(null);
const answers = reactive<Record<string, string>>({});
const asked = ref<MemoryClarification[]>([]);
let controller: AbortController | null = null;

const busy = computed(() => store.isPending.value);
const storeError = computed(() => (store.isError.value ? errorText(store.error.value) : ''));
const challenged = computed(() => result.value?.status === 'challenged');
// The text field holds one fact, so only its challenge applies; lines mode shows every challenge.
const shownChallenges = computed(() =>
  (result.value?.challenges ?? []).filter((c) => fromLines.value || c.index === 0),
);
const stored = computed(() => result.value?.status === 'stored');
const projectKey = computed(() => (scope.value === 'project' ? project.value.trim() : ''));
const fromLines = computed(() => props.lines !== undefined);
const canSubmit = computed(
  () =>
    !busy.value &&
    (fromLines.value ? (props.lines?.length ?? 0) > 0 : text.value.trim() !== '') &&
    (scope.value !== 'project' || projectKey.value !== ''),
);

function answerKey(index: number, q: number): string {
  return `${index}:${q}`;
}

async function submit(): Promise<void> {
  const clarifications = [...asked.value];
  for (const c of result.value?.challenges ?? []) {
    c.questions.forEach((question, q) => {
      const answer = (answers[answerKey(c.index, q)] ?? '').trim();
      if (answer !== '') clarifications.push({ question, answer });
    });
  }
  controller = new AbortController();
  try {
    const res = await store.mutateAsync({
      items: props.lines
        ? props.lines.map((fact) => ({ fact, reason: distilledReason(props.source ?? '') }))
        : [{ fact: text.value.trim(), reason: MANUAL_REASON }],
      clarifications,
      scope: scope.value,
      project: projectKey.value,
      signal: controller.signal,
    });
    asked.value = clarifications;
    result.value = res;
  } catch {
    // Surfaced through store.error; an aborted call needs nothing.
  } finally {
    controller = null;
  }
}

function cancel(): void {
  controller?.abort();
  store.reset();
}

function actionLabel(o: MemoryStoreResult['outcomes'][number]): string {
  switch (o.action) {
    case 'add':
      return t('common.added.status');
    case 'update':
      return t('memory.add.outcome.updated.status', { from: o.version - 1, to: o.version });
    case 'noop':
      return t('memory.add.outcome.known.status');
    default:
      return t('memory.add.outcome.failed.error', { error: o.error });
  }
}

function select(id: string): void {
  if (id === '') return;
  ui.selectedId = id;
  emit('close');
}
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent
      size="lg"
      :dirty="!stored && !fromLines && text.trim() !== ''"
      data-testid="add-memory-dialog"
    >
      <DialogHeader>
        <DialogTitle>{{ t('memory.add.label') }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <Alert v-if="storeError" variant="error" data-testid="add-memory-error">
          <AlertDescription>{{ storeError }}</AlertDescription>
        </Alert>

        <template v-if="stored && result">
          <ul class="m-0 flex list-none flex-col gap-1 p-0" data-testid="add-memory-outcomes">
            <li v-for="(o, i) in result.outcomes" :key="i">
              <Card>
                <span
                  class="text-kira-sm text-muted-foreground"
                  :data-testid="`add-memory-outcome-${i}`"
                  >{{
                    actionLabel(o)
                  }}</span
                >
                <button
                  v-if="o.action !== 'failed'"
                  type="button"
                  class="cursor-default border-0 bg-transparent p-0 text-left text-kira-md hover:underline"
                  @click="select(o.id)"
                  >{{
                    o.fact
                  }}</button
                >
                <span v-else class="text-kira-md">{{ o.fact }}</span>
              </Card>
            </li>
          </ul>
        </template>

        <template v-else>
          <div class="flex flex-col gap-1">
            <template v-if="lines">
              <Label>{{ t('memory.distill.store.items.label', { count: lines.length }) }}</Label>
              <ul
                class="m-0 flex max-h-48 list-none flex-col gap-0.5 overflow-y-auto p-0 text-kira-md"
                data-testid="add-memory-lines"
              >
                <li v-for="(line, i) in lines" :key="i" class="whitespace-pre-wrap">{{ line }}</li>
              </ul>
            </template>
            <Field v-else>
              <FieldLabel for="memory-text">{{ t('memory.add.text.label') }}</FieldLabel>
              <Textarea
                id="memory-text"
                v-model="text"
                :disabled="busy"
                :maxlength="MAX_TEXT"
                :placeholder="t('memory.add.text.placeholder')"
                data-testid="add-memory-text"
              />
              <span
                class="self-end text-kira-sm text-muted-foreground"
                data-testid="add-memory-count"
                >{{ text.length }}
                / {{ MAX_TEXT }}</span
              >
            </Field>
            <ScopeField
              v-model:scope="scope"
              v-model:project="project"
              prefix="add-memory"
              :scopes="SCOPES"
            />
            <template v-if="challenged">
              <Alert
                v-for="c in shownChallenges"
                :key="c.index"
                variant="warn"
                :icon="null"
                data-testid="add-memory-challenge"
              >
                <span v-if="lines" class="text-kira-sm font-medium">{{ lines[c.index] }}</span>
                <Field v-for="(question, q) in c.questions" :key="q">
                  <FieldLabel :for="`memory-answer-${q}`">{{ question }}</FieldLabel>
                  <Textarea
                    :id="`memory-answer-${q}`"
                    v-model="answers[answerKey(c.index, q)]"
                    :disabled="busy"
                    :placeholder="t('memory.add.answer.placeholder')"
                    data-testid="add-memory-answer"
                  />
                </Field>
              </Alert>
            </template>
          </div>
          <FieldDescription v-if="busy" data-testid="add-memory-busy">{{
            t('memory.add.checking.loading')
          }}</FieldDescription>
        </template>
      </DialogBody>
      <DialogFooter>
        <template v-if="stored">
          <Button
            variant="dialog-primary"
            size="kira-lg"
            data-testid="add-memory-done"
            @click="emit('close')"
            >{{
              t('common.done.status')
            }}</Button
          >
        </template>
        <template v-else>
          <Button
            v-if="busy"
            variant="dialog"
            size="kira-lg"
            data-testid="add-memory-cancel"
            @click="cancel"
            >{{
              t('common.action.cancel.label')
            }}</Button
          >
          <Button v-else variant="dialog" size="kira-lg" @click="emit('close')">{{
            t('common.action.close.label')
          }}</Button>
          <Button
            variant="dialog-primary"
            size="kira-lg"
            :disabled="!canSubmit"
            data-testid="add-memory-submit"
            @click="submit"
          >
            {{ challenged ? t('memory.add.resubmit.label') : t('common.action.add.label') }}
          </Button>
        </template>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
