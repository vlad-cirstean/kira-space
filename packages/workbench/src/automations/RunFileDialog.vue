<script setup lang="ts">
import type { RunFile, ScriptParam, SecretValues } from '@shared/domain/scripts';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Field, FieldError, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { useAgentInfo } from '@workbench/agent/agentInfo';
import { configName } from '@workbench/config/configName';
import { computed, ref, watch } from 'vue';
import ConfigFileDialog from '../config/ConfigFileDialog.vue';
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
const title = computed(() => script.value?.name ?? configName(props.file));

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
const hasSecrets = computed<boolean>(() => Object.keys(liveSecrets.value).length > 0);

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
  secrets: { pending: (): boolean => hasSecrets.value, flush: flushSecrets },
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
  params: () => t('common.field.parameters.label'),
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

const POINTERS = { ...RUN_TAB_POINTERS, folder: ['/dir', '/path'] } as const;
// Client-side checks, on top of the server problem's pointer count.
function clientErrors(s: Section): number {
  const e = errs.value;
  if (!e) return 0;
  if (s === 'smart') return e.budget ? 1 : 0;
  if (s === 'params') return tabErrorCount(e, 'params');
  if (s === 'schedule') return tabErrorCount(e, 'schedule');
  return 0;
}
const items = computed(() =>
  sections.value.map((id) => ({ value: id, label: LABEL[id](), testid: `run-file-tab-${id}` })),
);
</script>

<template>
  <ConfigFileDialog
    v-model:section="section"
    :draft="draft"
    :query="query"
    :title="title"
    testid-prefix="run-file"
    :sections="items"
    :tabs-aria="t('automations.form.tabs.aria')"
    :yaml-aria="t('automations.file.field.aria')"
    :pointers="POINTERS"
    :extra-errors="clientErrors"
    :last-good="view?.lastGood"
    :last-good-hint="t('automations.file.lastGood.hint')"
    :problem="view?.problem"
    :editable="view?.editable ?? true"
    @close="emit('close')"
  >
    <template v-if="isSmart" #description><SmartBadge /></template>
    <template #form>
      <template v-if="view && draft.model.value">
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
            <Checkbox id="script-schedule" v-model="scheduleOn" data-testid="script-schedule" />
            <FieldLabel for="script-schedule">{{
              t('automations.schedule.enable.label')
            }}</FieldLabel>
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
      </template>
    </template>
    <template #yaml-after>
      <RunSecretsSection v-if="script" :script="script" />
    </template>
  </ConfigFileDialog>
</template>
