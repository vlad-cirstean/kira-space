<script setup lang="ts">
import { useMutation, useQuery } from '@tanstack/vue-query';
import CodeBlock from '@theme/components/CodeBlock.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { Field, FieldError, FieldLabel } from '@theme/components/ui/field';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import type { TextPart } from '@theme/varText';
import { refDebounced } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import QueryState from '@workbench/components/QueryState.vue';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { computed, ref, watch } from 'vue';
import { errorText } from '../../bridge/codedError';
import { useAutomationsModule } from '../module';
import ParamsForm from '../run/ParamsForm.vue';
import RunPreview from '../run/RunPreview.vue';
import { useScriptRuns } from '../runs/runsQueries';
import { useScriptTabs } from '../scriptTabs';
import SmartBadge from '../smart/SmartBadge.vue';
import { clockText } from './scheduleText';

// Asks before a scheduled run: a waiting run (`runId`) or a Run now request. Shows the resolved run
// (every variable through VarText) and the secret params the schedule never stores.
const props = defineProps<{ scriptId: string; runId: string | null; more: number }>();
const emit = defineEmits<{ close: [] }>();
const ctx = useAutomationsModule();
const scriptTabs = useScriptTabs();

const script = computed(() => ctx.scripts.records().find((s) => s.id === props.scriptId) ?? null);
const secretParams = computed(() => (script.value?.params ?? []).filter((p) => p.secret));
const secrets = ref<Record<string, string[]>>({});
const dirty = computed(() => Object.values(secrets.value).some((v) => v.some((x) => x !== '')));
const debounced = refDebounced(secrets, TIMING.preview);

// Secret values stay out of the query key; a change refetches by hand.
const preview = useQuery({
  queryKey: ['scriptRuns', 'schedulePreview', props.scriptId, props.runId ?? 'now'],
  queryFn: () => ctx.runs.schedulePreview(props.scriptId, secrets.value),
  staleTime: 0,
  gcTime: 0,
});
watch(debounced, () => void preview.refetch());
const pv = computed(() => preview.data.value ?? null);
const settled = computed(
  () =>
    JSON.stringify(debounced.value) === JSON.stringify(secrets.value) && !preview.isFetching.value,
);

const { data: runs } = useScriptRuns();
const waitingRun = computed(() => runs.value?.find((r) => r.id === props.runId) ?? null);
const due = computed(() => {
  const at = waitingRun.value?.createdAt;
  const tz = script.value?.schedule?.timezone ?? 'UTC';
  return at === undefined
    ? t('common.now.status')
    : t('automations.schedule.due.status', { time: clockText(at, tz) });
});

const error = ref<string | null>(null);
const accept = useMutation({
  mutationKey: ['scriptRuns', 'confirmAccept'],
  mutationFn: async () => {
    const current = pv.value;
    if (!current) throw new Error(t('automations.error.nothingToRun.error'));
    return props.runId
      ? ctx.runs.confirmAccept(props.runId, current.hash, secrets.value)
      : ctx.runs.runScheduleNow(props.scriptId, current.hash, secrets.value);
  },
  onSuccess: (started) => {
    const sched = script.value?.schedule;
    scriptTabs.openRun({
      scriptId: props.scriptId,
      taskId: sched?.taskId ?? '',
      branchId: sched?.branchId ?? '',
      runId: started.runId,
      label: script.value?.name ?? t('common.script.label'),
    });
    emit('close');
  },
  onError: (err) => {
    error.value = errorText(err);
    void preview.refetch();
  },
});
const decline = useMutation({
  mutationKey: ['scriptRuns', 'confirmDecline'],
  mutationFn: async () => {
    if (props.runId) await ctx.runs.confirmDecline(props.runId);
  },
  onSuccess: () => emit('close'),
  onError: (err) => {
    error.value = errorText(err);
  },
});

const canRun = computed(
  () =>
    pv.value !== null &&
    settled.value &&
    pv.value.missing.length === 0 &&
    pv.value.blocker === '' &&
    !accept.isPending.value,
);

const promptParts = computed<TextPart[]>(() =>
  (pv.value?.prompt ?? []).map((p) => (p.var === '' ? p.text : { name: p.var, value: p.value })),
);
const ade = computed(() => {
  const a = pv.value?.ade;
  return a && a.taskId !== '' ? a : null;
});

function run(): void {
  if (!canRun.value) return;
  error.value = null;
  accept.mutate();
}

const onSubmitKey = shortcutKeydown(['form.submit'], run);
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent @keydown="onSubmitKey" size="lg" :dirty="dirty" data-testid="schedule-confirm">
      <DialogHeader close-testid="schedule-confirm-close">
        <DialogTitle>{{
          script
            ? t('automations.dialog.scheduleConfirm.title', { name: script.name })
            : t('automations.dialog.scheduleConfirm.untitledConfirm.title')
        }}</DialogTitle>
        <SmartBadge v-if="script?.kind === 'smart'" />
        <span class="text-kira-sm text-muted-foreground" data-testid="schedule-confirm-due">{{
          due
        }}</span>
      </DialogHeader>

      <DialogBody>
        <Alert v-if="!script" variant="error">
          <AlertDescription>{{ t('automations.script.gone.error') }}</AlertDescription>
        </Alert>
        <template v-else>
          <div
            v-if="ade"
            class="text-kira-sm text-muted-foreground"
            data-testid="schedule-confirm-ade"
          >
            <CatalogText
              :k="
                ade.branchLabel
                  ? 'automations.run.taskOnBranch.label'
                  : 'automations.run.taskOnly.label'
              "
            >
              <template #task
                ><VarText :parts="[{ name: 'task', value: ade.taskTitle }]" /></template
              >
              <template #branch
                ><VarText
                  :parts="[{ name: 'branch', value: ade.branchLabel ?? '', kind: 'branch' }]"
                /></template
              >
            </CatalogText>
          </div>
          <Field v-if="script.kind === 'smart' && pv" data-testid="schedule-confirm-prompt">
            <FieldLabel>{{ t('common.prompt.label') }}</FieldLabel>
            <CodeBlock>
              <VarText :parts="promptParts" />
            </CodeBlock>
            <CodeBlock class="text-muted-foreground">{{ pv.suffix }}</CodeBlock>
          </Field>
          <ParamsForm
            v-if="secretParams.length > 0"
            v-model="secrets"
            :params="secretParams"
            :saved="script.savedSecrets"
          />
          <QueryState :query="preview" variant="inline" :is-empty="() => false">
            <template #loading />
            <template #error="{ message }">
              <Alert variant="error" data-testid="schedule-confirm-preview-error">
                <AlertDescription>{{ message }}</AlertDescription>
              </Alert>
            </template>
            <template #default="{ data }">
              <RunPreview :preview="data" />
            </template>
          </QueryState>
          <FieldError v-if="pv && pv.missing.length > 0" data-testid="schedule-confirm-missing">
            {{ t('automations.run.fillIn.error', { names: pv.missing.join(', ') }) }}
          </FieldError>
          <FieldError v-if="pv?.blocker" data-testid="schedule-confirm-blocker">{{
            pv.blocker
          }}</FieldError>
          <Alert v-if="error" variant="error" data-testid="schedule-confirm-error">
            <AlertDescription>{{ error }}</AlertDescription>
          </Alert>
        </template>
      </DialogBody>

      <DialogFooter>
        <template v-if="more > 0" #start>
          <span class="text-kira-sm text-muted-foreground" data-testid="schedule-confirm-more">{{
            t('common.moreWaiting.status', { count: more })
          }}</span>
        </template>
        <Button
          variant="dialog"
          size="kira-lg"
          :pending="decline.isPending.value"
          data-testid="schedule-confirm-decline"
          @click="runId ? decline.mutate() : emit('close')"
        >
          {{ t('git.dialog.postCheckoutPullConfirm.notNow.label') }}
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!canRun"
          :pending="accept.isPending.value"
          data-testid="schedule-confirm-run"
          @click="run"
        >
          {{ t('common.action.run.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
