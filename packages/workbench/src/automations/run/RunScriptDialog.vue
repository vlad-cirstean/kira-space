<script setup lang="ts">
import type { ScriptRunArgs } from '@shared/domain/scriptRuns';
import { keepPreviousData, useMutation, useQuery } from '@tanstack/vue-query';
import CodeBlock from '@theme/components/CodeBlock.vue';
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
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Textarea } from '@theme/components/ui/textarea';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import type { TextPart } from '@theme/varText';
import { refDebounced } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import QueryState from '@workbench/components/QueryState.vue';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { computed, ref } from 'vue';
import { errorText } from '../../bridge/codedError';
import { useAutomationsModule } from '../module';
import { useScriptTabs } from '../scriptTabs';
import SmartBadge from '../smart/SmartBadge.vue';
import AdeContextFields from './AdeContextFields.vue';
import ParamsForm from './ParamsForm.vue';
import RunPreview from './RunPreview.vue';

const props = defineProps<{
  scriptId: string;
  prefill?: Record<string, string[]>;
  taskId?: string;
  branchId?: string;
  from?: 'automations' | 'ade';
}>();
const emit = defineEmits<{ close: [] }>();
const ctx = useAutomationsModule();
const scriptTabs = useScriptTabs();

const script = computed(() => ctx.scripts.records().find((s) => s.id === props.scriptId) ?? null);
const isSmart = computed(() => script.value?.kind === 'smart');

const values = ref<Record<string, string[]>>(
  Object.fromEntries(
    (script.value?.params ?? []).map((p) => [p.name, props.prefill?.[p.name] ?? [...p.default]]),
  ),
);
// null: use the saved prompt. A string is this run's own prompt; never saved.
const oneOff = ref<string | null>(null);
const taskId = ref(props.taskId ?? '');
const branchId = ref(props.branchId ?? '');

const initialValues = JSON.stringify(values.value);
const dirty = computed(
  () => oneOff.value !== null || JSON.stringify(values.value) !== initialValues,
);

const args = computed<ScriptRunArgs>(() => ({
  scriptId: props.scriptId,
  params: values.value,
  prompt: oneOff.value,
  taskId: taskId.value,
  branchId: branchId.value,
}));
function runKey() {
  return { scriptId: props.scriptId, taskId: taskId.value, branchId: branchId.value };
}
const debounced = refDebounced(args, TIMING.preview);

const preview = useQuery({
  queryKey: ['scriptRuns', 'preview', debounced],
  queryFn: () => ctx.runs.preview(debounced.value),
  placeholderData: keepPreviousData,
  staleTime: 0,
});
const pv = computed(() => preview.data.value ?? null);
const settled = computed(
  () => JSON.stringify(debounced.value) === JSON.stringify(args.value) && !preview.isFetching.value,
);

const startError = ref<string | null>(null);
const start = useMutation({
  mutationKey: ['scriptRuns', 'start'],
  mutationFn: async () => {
    const current = pv.value;
    if (!current) throw new Error(t('automations.error.nothingToRun.error'));
    if (scriptTabs.focusLive(runKey())) return null;
    return ctx.runs.start(args.value, current.hash);
  },
  onSuccess: (started) => {
    if (started === null) {
      emit('close');
      return;
    }
    const s = script.value;
    if (!s) return;
    if (started.terminal) {
      if (props.from === 'ade') ctx.showAutomations();
      ctx.openTerminalTab({
        cwd: started.terminal.cwd,
        launch: {
          command: s.command,
          label: s.name,
          color: s.color,
          kind: 'script',
          scriptId: s.id,
          launchToken: started.terminal.token,
          taskId: taskId.value,
          branchId: branchId.value,
        },
        replace: scriptTabs.replaceTarget(runKey()),
      });
    } else {
      scriptTabs.openRun({ ...runKey(), runId: started.runId, label: s.name });
    }
    emit('close');
  },
  onError: (err) => {
    startError.value = errorText(err);
    void preview.refetch();
  },
});

const canRun = computed(
  () =>
    pv.value !== null &&
    settled.value &&
    pv.value.missing.length === 0 &&
    pv.value.blocker === '' &&
    !start.isPending.value,
);

const promptParts = computed<TextPart[]>(() =>
  (pv.value?.prompt ?? []).map((p) => (p.var === '' ? p.text : { name: p.var, value: p.value })),
);

function editPrompt(): void {
  oneOff.value = pv.value?.body ?? '';
}

function run(): void {
  if (!canRun.value) return;
  startError.value = null;
  start.mutate();
}

const onSubmitKey = shortcutKeydown(['form.submit'], run);
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent @keydown="onSubmitKey" size="lg" :dirty="dirty" data-testid="run-dialog">
      <DialogHeader>
        <DialogTitle>{{
          script
            ? t('automations.dialog.run.title', { name: script.name })
            : t('automations.dialog.run.untitled.title')
        }}</DialogTitle>
        <SmartBadge v-if="isSmart" />
      </DialogHeader>

      <DialogBody>
        <Alert v-if="!script" variant="error">
          <AlertDescription>{{ t('automations.script.gone.error') }}</AlertDescription>
        </Alert>
        <template v-else>
          <ParamsForm
            v-if="script.params.length > 0"
            v-model="values"
            :params="script.params"
            :saved="script.savedSecrets"
          />

          <Field v-if="isSmart && pv" data-testid="run-prompt">
            <FieldLabel>{{ t('common.prompt.label') }}</FieldLabel>
            <template v-if="oneOff === null">
              <CodeBlock data-testid="run-prompt-text">
                <VarText :parts="promptParts" />
              </CodeBlock>
              <Button
                variant="dialog"
                size="kira-lg"
                class="self-start"
                data-testid="run-prompt-edit"
                @click="editPrompt"
              >
                {{ t('automations.dialog.run.editPrompt.label') }}
              </Button>
            </template>
            <template v-else>
              <Textarea
                :model-value="oneOff"
                rows="6"
                spellcheck="false"
                class="field-sizing-fixed min-h-32 resize-y font-data leading-normal"
                data-testid="run-prompt-input"
                @update:model-value="(v) => (oneOff = String(v))"
              />
              <div class="flex items-center gap-2">
                <Button
                  variant="dialog"
                  size="kira-lg"
                  data-testid="run-prompt-reset"
                  @click="oneOff = null"
                  >{{
                    t('common.action.reset.label')
                  }}</Button
                >
                <FieldDescription>{{ t('automations.dialog.run.notSaved.hint') }}</FieldDescription>
              </div>
            </template>
            <FieldLabel class="mt-1 text-muted-foreground">{{
              t('automations.dialog.run.suffix.label')
            }}</FieldLabel>
            <CodeBlock class="text-muted-foreground" data-testid="run-suffix">{{
              pv.suffix
            }}</CodeBlock>
          </Field>

          <AdeContextFields
            v-if="pv && (pv.needs.tasks.length > 0 || pv.needs.branches.length > 0 || pv.ade)"
            v-model:task-id="taskId"
            v-model:branch-id="branchId"
            :needs="pv.needs"
            :ade="pv.ade"
          />

          <QueryState :query="preview" variant="inline" :is-empty="() => false">
            <template #loading />
            <template #error="{ message }">
              <Alert variant="error" data-testid="run-preview-error">
                <AlertDescription>{{ message }}</AlertDescription>
              </Alert>
            </template>
            <template #default="{ data }">
              <RunPreview :preview="data" />
            </template>
          </QueryState>
          <FieldError v-if="pv && pv.missing.length > 0" data-testid="run-missing">
            {{ t('automations.run.fillIn.error', { names: pv.missing.join(', ') }) }}
          </FieldError>
          <FieldError v-if="pv?.blocker" data-testid="run-blocker">{{ pv.blocker }}</FieldError>
          <Alert v-if="startError" variant="error" data-testid="run-error">
            <AlertDescription>{{ startError }}</AlertDescription>
          </Alert>
        </template>
      </DialogBody>

      <DialogFooter>
        <DialogClose as-child>
          <Button variant="dialog" size="kira-lg" data-testid="run-cancel">{{
            t('common.action.cancel.label')
          }}</Button>
        </DialogClose>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!canRun"
          data-testid="run-start"
          @click="run"
        >
          {{ t('common.action.run.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
