<script setup lang="ts">
import type { RunFile, ScriptParam, SecretValues } from '@shared/domain/scripts';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { Field, FieldError } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { useAgentInfo } from '@workbench/agent/agentInfo';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import CodeTextEditor from '@workbench/editor/CodeTextEditor.vue';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { computed, ref, watch } from 'vue';
import { errorText } from '../bridge/codedError';
import ConfigFileBar from '../config/ConfigFileBar.vue';
import { useConfigFormDraft } from '../config/useConfigFormDraft';
import McpToolsField from './form/McpToolsField.vue';
import ParamsEditor from './form/ParamsEditor.vue';
import RunFolderTab from './form/RunFolderTab.vue';
import RunMcpTab from './form/RunMcpTab.vue';
import RunScriptTab from './form/RunScriptTab.vue';
import {
  fileParamsOf,
  fileScheduleOf,
  fileSmartOf,
  paramsOf,
  RUN_FIELDS,
  RUN_TAB_POINTERS,
  scheduleOf,
  smartOf,
} from './form/runForm';
import ScheduleFields from './form/ScheduleFields.vue';
import SmartSettingsFields from './form/SmartSettingsFields.vue';
import { type EditorTab, editorErrors, tabErrorCount } from './form/scriptErrors';
import ToolsField from './form/ToolsField.vue';
import { useAutomationsModule } from './module';
import RunSecretsSection from './RunSecretsSection.vue';
import { useRunFile, useRunForm, useSaveRunFile } from './runFileQueries';
import { newSchedule } from './schedule/scheduleText';
import SmartBadge from './smart/SmartBadge.vue';
import { defaultSmart } from './smart/smartSettings';

// One run file, as a form and as YAML over the same draft text. Form edits become node edits on the
// server (comments and key order stay), a YAML edit re-decodes into the form. Save sends the hash of
// the text the draft was edited from; a file changed elsewhere is refused with Reload and Overwrite.
const props = defineProps<{ file: string }>();
const emit = defineEmits<{ close: [] }>();

type Section = EditorTab | 'smart' | 'folder' | 'mcp' | 'yaml';

const ctx = useAutomationsModule();
const { info: agentInfo } = useAgentInfo();
const query = useRunFile(() => props.file);
const save = useSaveRunFile();
const applyForm = useRunForm(() => props.file);

const view = computed(() => query.data.value);
const script = computed(() => ctx.scripts.records().find((s) => s.id === view.value?.id));
const title = computed(() => script.value?.name ?? props.file);

// Write-only typed values of secret params, component-local so they die with the dialog.
const secretDraft = ref<SecretValues>({});
const liveSecrets = computed<SecretValues>(() => {
  const live = new Set(
    paramsOf(draft.model.value ?? { name: '' })
      .filter((p) => p.secret && p.type === 'text')
      .map((p) => p.name),
  );
  return Object.fromEntries(Object.entries(secretDraft.value).filter(([n]) => live.has(n)));
});
const hasSecrets = computed(() => Object.keys(liveSecrets.value).length > 0);

async function flushSecrets(): Promise<void> {
  const id = view.value?.id;
  const values = liveSecrets.value;
  if (!id || Object.keys(values).length === 0) return;
  await ctx.scripts.setSecrets(id, values);
  secretDraft.value = {};
}

const draft = useConfigFormDraft<RunFile>({
  file: () => (view.value ? { text: view.value.text, hash: view.value.hash } : undefined),
  form: applyForm,
  save: (text, baseHash) => save.mutateAsync({ file: props.file, text, baseHash }),
  refetch: async () => {
    const fresh = (await query.refetch()).data;
    return fresh ? { text: fresh.text, hash: fresh.hash } : undefined;
  },
  fields: RUN_FIELDS,
  afterSave: flushSecrets,
});

// A change on disk refreshes the file while no edits are pending.
watch(
  () =>
    ctx.scripts
      .records()
      .map((s) => s.updatedAt)
      .join('|') + ctx.scripts.problems().length,
  () => {
    if (!draft.dirty.value) void query.refetch();
  },
);

const section = ref<Section>('script');
watch(section, (s) => {
  void draft.showTab(s === 'yaml' ? 'yaml' : 'form');
});

const isSmart = computed(() => draft.model.value?.kind === 'smart');
const sections = computed<Section[]>(() => [
  'script',
  ...(isSmart.value ? (['smart'] as const) : []),
  'folder',
  'schedule',
  'params',
  ...(ctx.mcp && !isSmart.value ? (['mcp'] as const) : []),
  'yaml',
]);
const LABEL = {
  script: () => t('common.script.label'),
  smart: () => t('common.smart.label'),
  folder: () => t('automations.form.folder.label'),
  schedule: () => t('automations.schedule.title.label'),
  params: () => t('automations.params.title.label'),
  mcp: () => t('common.mcp.label'),
  yaml: () => t('common.yaml.label'),
} as const satisfies Record<Section, () => string>;

// Domain-shaped views of the model's lists, for the restored field components.
const params = computed<ScriptParam[]>({
  get: () => (draft.model.value ? paramsOf(draft.model.value) : []),
  set: (v) => {
    if (draft.model.value) draft.model.value.params = fileParamsOf(v);
  },
});
const smart = computed({
  get: () =>
    draft.model.value
      ? smartOf(draft.model.value, defaultSmart(agentInfo.value.defaultModel))
      : defaultSmart(''),
  set: (v) => {
    if (draft.model.value) draft.model.value.smart = fileSmartOf(v);
  },
});
const schedule = computed({
  get: () => (draft.model.value ? scheduleOf(draft.model.value) : null),
  set: (v) => {
    if (!draft.model.value) return;
    draft.model.value.schedule = v ? fileScheduleOf(v) : undefined;
  },
});
const scheduleOn = computed({
  get: () => schedule.value !== null,
  set: (on: boolean) => {
    schedule.value = on ? newSchedule() : null;
  },
});
const scheduleOk = ref(true);

const savedSecrets = computed(() => script.value?.savedSecrets ?? []);
const willBeSaved = computed(() => {
  const typed = liveSecrets.value;
  const kept = savedSecrets.value.filter((n) => typed[n] !== null);
  return [...new Set([...kept, ...Object.keys(typed).filter((n) => typeof typed[n] === 'string')])];
});
const storageOff = computed(() => !ctx.scripts.secretStorage().available);

const errs = computed(() => {
  const m = draft.model.value;
  if (!m) return null;
  const s = smart.value;
  return editorErrors(
    {
      kind: isSmart.value ? 'smart' : 'script',
      name: m.name,
      body: (isSmart.value ? m.prompt : m.command) ?? '',
      dirMode: m.dir === 'fixed' ? 'fixed' : 'kira',
      workingDir: m.path ?? '',
      maxBudgetUsd: s.maxBudgetUsd,
      noCostCap: s.noCostCap,
      params: params.value,
      schedule: schedule.value,
      scheduleOk: scheduleOk.value,
    },
    { name: false, body: false, folder: false },
  );
});

function serverCount(section: keyof typeof RUN_TAB_POINTERS): number {
  return RUN_TAB_POINTERS[section].some((p) => draft.errorFor(p)) ? 1 : 0;
}
function errorCount(s: Section): number {
  if (s === 'yaml') return 0;
  const e = errs.value;
  if (s === 'smart') return e?.budget ? 1 : 0;
  if (s === 'folder') return ['/dir', '/path'].some((p) => draft.errorFor(p)) ? 1 : 0;
  if (s === 'script') return serverCount('script');
  if (s === 'mcp') return serverCount('mcp');
  if (s === 'params') return (e ? tabErrorCount(e, 'params') : 0) + serverCount('params');
  return (e ? tabErrorCount(e, 'schedule') : 0) + serverCount('schedule');
}
const items = computed(() =>
  sections.value.map((id) => ({ value: id, label: LABEL[id](), testid: `run-file-tab-${id}` })),
);

const saveBusy = computed(() => draft.saving.value);
const canSave = computed(() => draft.canSave.value || hasSecrets.value);
const secretError = ref('');
const saveError = computed(() => draft.saveError.value || secretError.value);

async function saveNow(): Promise<void> {
  secretError.value = '';
  await draft.saveNow();
  if (draft.dirty.value || draft.saveError.value) return;
  try {
    await flushSecrets();
  } catch (err) {
    secretError.value = errorText(err);
  }
}

const onSubmitKey = shortcutKeydown(['form.submit'], () => void saveNow());
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent
      size="lg"
      data-testid="run-file-dialog"
      :dirty="draft.dirty.value"
      @keydown="onSubmitKey"
    >
      <DialogHeader close-testid="run-file-close">
        <DialogTitle>{{ title }}</DialogTitle>
        <SmartBadge v-if="isSmart" />
      </DialogHeader>
      <ViewToolbar v-if="view?.editable" class="px-3">
        <SecondaryTabs
          :model-value="section"
          :items="items"
          :aria-label="t('automations.form.tabs.aria')"
          @update:model-value="(v) => (section = v as Section)"
        >
          <template #item="{ item }">
            {{ item.label }}
            <Badge
              v-if="errorCount(item.value as Section) > 0"
              variant="error"
              :data-testid="`run-file-tab-${item.value}-errors`"
              >{{
                errorCount(item.value as Section)
              }}</Badge
            >
          </template>
        </SecondaryTabs>
      </ViewToolbar>
      <DialogBody class="min-h-80">
        <div v-if="view" class="flex min-w-0 flex-col gap-3">
          <ConfigFileBar
            :path="view.path"
            :error="view.problem"
            :open="() => ctx.files.open(view!.file)"
            :reveal="() => ctx.files.reveal(view!.file)"
            :about="() => ctx.files.open('AGENTS.md')"
          />
          <div
            v-if="view.lastGood"
            class="text-kira-sm text-subtle"
            data-testid="run-file-last-good"
          >
            {{ t('automations.file.lastGood.hint') }}
          </div>
          <div
            v-if="!view.editable"
            class="text-kira-sm text-subtle"
            data-testid="run-file-read-only"
          >
            {{ t('space.ade.context.readOnly.hint') }}
          </div>
          <template v-else>
            <template v-if="section !== 'yaml'">
              <Alert v-if="draft.problem.value" variant="error" data-testid="run-form-problem">
                <AlertDescription>{{
                  t('common.configForm.problem.status', {
                    line: draft.problem.value?.line ?? 0,
                    message: draft.problem.value?.message ?? '',
                  })
                }}</AlertDescription>
                <AlertAction>
                  <Button
                    variant="link"
                    size="inline"
                    data-testid="run-form-show-yaml"
                    @click="section = 'yaml'"
                  >
                    {{ t('common.configForm.showYaml.label') }}
                  </Button>
                </AlertAction>
              </Alert>
              <fieldset
                v-if="draft.model.value"
                :disabled="draft.formDisabled.value"
                class="m-0 flex min-w-0 flex-col gap-3 border-0 p-0"
              >
                <RunScriptTab
                  v-show="section === 'script'"
                  v-model="draft.model.value"
                  :saved="willBeSaved"
                  :error-for="draft.errorFor"
                />
                <div v-if="isSmart" v-show="section === 'smart'" class="flex flex-col gap-3">
                  <SmartSettingsFields v-model="smart" />
                  <FieldError v-if="errs?.budget" data-testid="script-dialog-budget-error">{{
                    errs.budget
                  }}</FieldError>
                  <ToolsField v-model="smart" />
                  <McpToolsField v-model="smart" />
                </div>
                <RunFolderTab
                  v-show="section === 'folder'"
                  v-model="draft.model.value"
                  :id="view.id"
                  :error-for="draft.errorFor"
                />
                <RunMcpTab
                  v-if="ctx.mcp && !isSmart"
                  v-show="section === 'mcp'"
                  v-model="draft.model.value"
                  :error-for="draft.errorFor"
                />
                <div v-show="section === 'schedule'" class="flex flex-col gap-3">
                  <Field orientation="horizontal">
                    <Checkbox
                      id="script-schedule"
                      v-model="scheduleOn"
                      data-testid="script-schedule"
                    />
                    <Label for="script-schedule">{{
                      t('automations.schedule.enable.label')
                    }}</Label>
                  </Field>
                  <ScheduleFields
                    v-if="schedule"
                    v-model="schedule"
                    :script-id="view.id"
                    :kind="isSmart ? 'smart' : 'script'"
                    :params="params"
                    :saved-secrets="willBeSaved"
                    @valid="(ok) => (scheduleOk = ok)"
                  />
                  <FieldError v-if="errs?.cron" data-testid="script-dialog-cron-error">{{
                    errs.cron
                  }}</FieldError>
                </div>
                <div v-show="section === 'params'" class="flex flex-col gap-3">
                  <ParamsEditor
                    v-model="params"
                    v-model:secret-values="secretDraft"
                    :kind="isSmart ? 'smart' : 'script'"
                    :errors="errs?.params"
                    :saved-secrets="savedSecrets"
                    :storage-off="storageOff"
                  />
                </div>
              </fieldset>
            </template>
            <template v-else>
              <CodeTextEditor
                :model-value="draft.draft.value"
                language="yaml"
                :problem="draft.problem.value"
                :ariaLabel="t('automations.file.field.aria')"
                class="h-64"
                data-testid="run-file-text"
                @update:model-value="draft.onYamlInput"
                @save="saveNow"
                @submit="saveNow"
              />
              <RunSecretsSection v-if="script" :script="script" />
            </template>
          </template>
          <Alert v-if="saveError" variant="error" data-testid="run-file-save-error">
            <AlertDescription>{{ saveError }}</AlertDescription>
            <AlertAction v-if="draft.conflict.value">
              <Button
                variant="link"
                size="inline"
                data-testid="run-file-reload"
                @click="draft.reload"
              >
                {{ t('common.action.refresh.label') }}
              </Button>
              <Button
                variant="link"
                size="inline"
                data-testid="run-file-overwrite"
                @click="draft.overwrite"
              >
                {{ t('space.ade.workflows.conflict.overwrite.label') }}
              </Button>
            </AlertAction>
          </Alert>
        </div>
        <div
          v-else-if="query.error.value"
          class="text-kira-sm text-tone-red"
          data-testid="run-file-error"
        >
          {{ errorText(query.error.value) }}
        </div>
      </DialogBody>
      <DialogFooter>
        <DialogClose as-child>
          <Button variant="dialog" size="kira-lg" data-testid="run-file-cancel">{{
            draft.dirty.value ? t('common.action.discard.label') : t('common.action.close.label')
          }}</Button>
        </DialogClose>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!canSave"
          :pending="saveBusy"
          data-testid="run-file-save"
          @click="saveNow"
        >
          {{ t('common.action.save.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
