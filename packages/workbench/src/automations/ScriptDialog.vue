<script setup lang="ts">
import { PALETTE_COLOR_CHOICES, type PaletteColor } from '@shared/domain/color';
import type {
  CustomScript,
  CustomScriptFields,
  ScriptDir,
  ScriptDirMode,
  ScriptKind,
  ScriptParam,
  ScriptSchedule,
  SecretValues,
  SmartSettings,
} from '@shared/domain/scripts';
import { useQuery } from '@tanstack/vue-query';
import PathChip from '@theme/components/PathChip.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
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
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { RadioGroup, RadioGroupItem } from '@theme/components/ui/radio-group';
import { Tabs, TabsContent } from '@theme/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import VarText from '@theme/components/VarText.vue';
import { type MessageKey, t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import SwatchRadio from '@theme/SwatchRadio.vue';
import type { TextPart } from '@theme/varText';
import { useFocus } from '@vueuse/core';
import { useAgentInfo } from '@workbench/agent/agentInfo';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { useFormErrors } from '@workbench/behaviour/formErrors';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { type ComponentPublicInstance, computed, ref, toRaw, useTemplateRef } from 'vue';
import { errorText } from '../bridge/codedError';
import type { ScriptsSeam } from './module';
import ParamsEditor from './ParamsEditor.vue';
import ScriptBodyField from './ScriptBodyField.vue';
import ScheduleFields from './schedule/ScheduleFields.vue';
import { newSchedule } from './schedule/scheduleText';
import { ALL_SHOWN, type EditorTab, editorErrors, tabErrorCount } from './scriptErrors';
import McpToolsField from './smart/McpToolsField.vue';
import SmartBadge from './smart/SmartBadge.vue';
import SmartSettingsFields from './smart/SmartSettingsFields.vue';
import { defaultSmart } from './smart/smartSettings';
import ToolsField from './smart/ToolsField.vue';

// Adds one script (`script === null`) or edits one. `scripts` is a prop, not
// `useAutomationsModule()`: AutomationsPanel.vue owns the seam and mounts this per open. `collectionId`
// presets the collection of a new script (added from a collection's own menu).
const props = defineProps<{
  scripts: ScriptsSeam;
  script: CustomScript | null;
  /** Kind of a new script; an edited script keeps its own. */
  kind?: ScriptKind;
  collectionId?: string | null;
  chooseFolder: (title: string) => Promise<string | null>;
  resolveDir: (scriptId: string) => Promise<ScriptDir>;
  /** Kira Space: offers the task-worktree switch. */
  ade?: boolean;
  /** Tab to open on; defaults to Script. */
  initialTab?: EditorTab;
}>();
const emit = defineEmits<{ close: [] }>();

const colors = PALETTE_COLOR_CHOICES;
const title = computed(() => {
  if (props.script)
    return t(
      isSmart
        ? 'automations.dialog.script.editSmart.title'
        : 'automations.dialog.script.edit.title',
    );
  return t(isSmart ? 'automations.script.newSmart.label' : 'automations.script.new.label');
});

const { info: agentInfo } = useAgentInfo();
const kind: ScriptKind = props.script?.kind ?? props.kind ?? 'script';
const isSmart = kind === 'smart';
const name = ref(props.script?.name ?? '');
const params = ref<ScriptParam[]>(structuredClone(toRaw(props.script?.params ?? [])));
const smart = ref<SmartSettings>(
  structuredClone(toRaw(props.script?.smart ?? defaultSmart(agentInfo.value.defaultModel))),
);
// null: not recurring. Turning the switch off keeps the edits until Save.
const schedule = ref<ScriptSchedule | null>(structuredClone(toRaw(props.script?.schedule ?? null)));
const scheduleOk = ref(true);
const scheduleOn = computed({
  get: () => schedule.value !== null,
  set: (on: boolean) => {
    if (!on) schedule.value = null;
    else schedule.value ??= newSchedule();
  },
});
const command = ref(props.script?.command ?? '');
const dirMode = ref<ScriptDirMode>(props.script?.dirMode ?? 'kira');
const workingDir = ref(props.script?.workingDir ?? '');
const useAdeDir = ref(props.script?.useAdeDir ?? true);
const dir = useQuery({
  queryKey: ['scriptRuns', 'dir', props.script?.id ?? ''],
  queryFn: () => props.resolveDir(props.script?.id ?? ''),
  staleTime: 0,
});
const blocker = computed(() =>
  dirMode.value === props.script?.dirMode ? (dir.data.value?.blocker ?? '') : '',
);
const folderParts = computed<TextPart[]>(() => [
  {
    name: 'folder',
    kind: 'path',
    value: `${dir.data.value?.base ?? '…'}/automations/${props.script?.id ?? t('automations.dialog.script.setOnSave.status')}`,
  },
]);
const homeParts = computed<TextPart[]>(() => [
  { name: 'HOME', value: dir.data.value?.base ?? '…', kind: 'path' },
]);
// '' is the select's "No collection" sentinel; the payload sends null.
const collection = ref(props.script?.collectionId ?? props.collectionId ?? '');
const color = ref<PaletteColor>((props.script?.color as PaletteColor | undefined) ?? 'none');
const error = ref<string | null>(null);

const TAB_IDS: readonly EditorTab[] = ['script', 'params', 'schedule'];
const TAB_LABEL_KEYS = {
  script: 'common.script.label',
  params: 'automations.params.title.label',
  schedule: 'automations.schedule.title.label',
} as const satisfies Record<EditorTab, MessageKey>;
const tab = ref<EditorTab>(props.initialTab ?? 'script');
const tabItems = computed(() =>
  TAB_IDS.map((id) => ({
    value: id,
    label: t(TAB_LABEL_KEYS[id]),
    testid: `script-dialog-tab-${id}`,
  })),
);

const draft = computed(() => ({
  kind,
  name: name.value,
  body: command.value,
  dirMode: dirMode.value,
  workingDir: workingDir.value,
  maxBudgetUsd: smart.value.maxBudgetUsd,
  params: params.value,
  schedule: schedule.value,
  scheduleOk: scheduleOk.value,
}));
const all = computed(() => editorErrors(draft.value, ALL_SHOWN));
const form = useFormErrors(
  {
    name: { value: () => name.value, error: () => all.value.name ?? null, tab: 'script' },
    body: { value: () => command.value, error: () => all.value.body ?? null, tab: 'script' },
    folder: { value: () => dirMode.value, error: () => all.value.folder ?? null, tab: 'script' },
  },
  { onTab: (id) => (tab.value = id as EditorTab) },
);
// Name, body and folder reveal on first edit or failed Save; the rest always show.
const errors = computed(() => ({
  ...all.value,
  name: form.errorFor('name') ?? undefined,
  body: form.errorFor('body') ?? undefined,
  folder: form.errorFor('folder') ?? undefined,
}));
const firstBadTab = computed(() => TAB_IDS.find((id) => tabErrorCount(all.value, id) > 0));

// Write-only draft, component-local so it dies with the dialog; entries follow live secret params.
const secretDraft = ref<SecretValues>({});
const secretValues = computed<SecretValues>(() => {
  const live = new Set(
    params.value.filter((p) => p.secret && p.type === 'text').map((p) => p.name),
  );
  return Object.fromEntries(Object.entries(secretDraft.value).filter(([n]) => live.has(n)));
});
const savedSecrets = computed(() => props.script?.savedSecrets ?? []);
// Saved names once this edit lands: kept saved ones plus newly typed, minus cleared.
const willBeSaved = computed(() => {
  const draft = secretValues.value;
  const kept = savedSecrets.value.filter((n) => draft[n] !== null);
  return [...new Set([...kept, ...Object.keys(draft).filter((n) => typeof draft[n] === 'string')])];
});
const storageOff = computed(() => !props.scripts.secretStorage().available);

const snapshot = (): string =>
  JSON.stringify([
    secretValues.value,
    name.value,
    command.value,
    params.value,
    smart.value,
    schedule.value,
    dirMode.value,
    workingDir.value,
    useAdeDir.value,
    collection.value,
    color.value,
  ]);
const initial = snapshot();
const dirty = computed(() => snapshot() !== initial);

async function chooseWorkingDir(): Promise<void> {
  error.value = null;
  try {
    const path = await props.chooseFolder(t('automations.dialog.script.chooseFolder.title'));
    if (path === null) return;
    workingDir.value = path;
    dirMode.value = 'fixed';
  } catch (err) {
    error.value = errorText(err);
  }
}

const persist = useAsyncAction(async (fields: CustomScriptFields) => {
  const values = Object.keys(secretValues.value).length > 0 ? secretValues.value : null;
  if (props.script) await props.scripts.update(props.script.id, fields, values);
  else await props.scripts.create(fields, values);
});
const saving = persist.pending;

async function save(): Promise<void> {
  if (saving.value) return;
  error.value = null;
  if (!(await form.submit())) return;
  if (firstBadTab.value !== undefined) {
    tab.value = firstBadTab.value;
    return;
  }
  const ok = await persist.run({
    name: name.value.trim(),
    command: command.value.trim(),
    kind,
    params: params.value,
    smart: isSmart ? smart.value : null,
    schedule: schedule.value,
    dirMode: dirMode.value,
    workingDir: dirMode.value === 'fixed' ? workingDir.value.trim() : '',
    color: color.value,
    collectionId: collection.value === '' ? null : collection.value,
    useAdeDir: useAdeDir.value,
  });
  if (ok) emit('close');
  else error.value = persist.error.value;
}

const nameEl = useTemplateRef<ComponentPublicInstance>('nameEl');
const { focused: nameFocused } = useFocus(nameEl);
// Reka's focus scope settles after mount; focusing from its open hook keeps the name field focused.
function onOpenAutoFocus(e: Event): void {
  e.preventDefault();
  nameFocused.value = true;
}
const onSubmitKey = shortcutKeydown(['form.submit'], () => void save());
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent
      @keydown="onSubmitKey"
      size="lg"
      data-testid="script-dialog"
      :dirty="dirty"
      @open-auto-focus="onOpenAutoFocus"
    >
      <DialogHeader close-testid="script-dialog-close">
        <DialogTitle>{{ title }}</DialogTitle>
        <SmartBadge v-if="isSmart" />
      </DialogHeader>

      <Tabs :model-value="tab" class="flex min-h-0 flex-1 flex-col gap-0">
        <ViewToolbar class="px-3">
          <SecondaryTabs
            :model-value="tab"
            :items="tabItems"
            :aria-label="t('automations.dialog.script.tabs.aria')"
            @update:model-value="(v) => (tab = v as typeof tab)"
          >
            <template #item="{ item }">
              {{ item.label }}
              <Badge
                v-if="tabErrorCount(errors, item.value as EditorTab) > 0"
                variant="error"
                :data-testid="`script-dialog-tab-${item.value}-errors`"
                >{{
                  tabErrorCount(errors, item.value as EditorTab)
                }}</Badge
              >
            </template>
          </SecondaryTabs>
        </ViewToolbar>

        <DialogBody class="min-h-80">
          <TabsContent
            value="script"
            force-mount
            class="flex flex-col gap-3 data-[state=inactive]:hidden"
          >
            <Field>
              <FieldLabel for="script-name">{{ t('common.field.name.label') }}</FieldLabel>
              <Input
                id="script-name"
                ref="nameEl"
                v-model="name"
                :placeholder="t('automations.dialog.script.nameExample.example')"
                v-bind="form.invalidAttrs('name')"
                data-testid="script-dialog-name"
              />
              <FieldError
                v-if="errors.name"
                :id="form.errorId('name')"
                data-testid="script-dialog-name-error"
                >{{
                  errors.name
                }}</FieldError
              >
            </Field>

            <div class="flex items-end gap-3">
              <Field class="flex-1 min-w-0">
                <FieldLabel for="script-collection">{{ t('common.collection.label') }}</FieldLabel>
                <NativeSelect
                  id="script-collection"
                  v-model="collection"
                  variant="bordered"
                  size="kira-lg"
                  data-testid="script-dialog-collection"
                >
                  <option value="">{{ t('automations.dialog.script.noCollection.option') }}</option>
                  <option v-for="c in scripts.collections()" :key="c.id" :value="c.id">{{
                    c.name
                  }}</option>
                </NativeSelect>
              </Field>
              <fieldset
                class="color-picker m-0 flex h-control flex-wrap items-center gap-1 border-0 p-0"
                :aria-label="t('common.field.colour.label')"
              >
                <Tooltip v-for="swatch in colors" :key="swatch">
                  <TooltipTrigger as-child>
                    <SwatchRadio
                      name="script-color"
                      :value="swatch"
                      :color="swatch"
                      :checked="color === swatch"
                      @change="color = swatch"
                    />
                  </TooltipTrigger>
                  <TooltipContent>{{
                    swatch === 'none' ? t('common.field.noColour.label') : swatch
                  }}</TooltipContent>
                </Tooltip>
              </fieldset>
            </div>

            <ScriptBodyField
              v-model="command"
              :kind="kind"
              :params="params"
              :ade="!!ade"
              :saved="willBeSaved"
              :error="errors.body"
              :error-id="form.errorId('body')"
            />

            <template v-if="isSmart">
              <SmartSettingsFields v-model="smart" />
              <FieldError v-if="errors.budget" data-testid="script-dialog-budget-error">{{
                errors.budget
              }}</FieldError>
              <ToolsField v-model="smart" />
              <McpToolsField v-model="smart" />
            </template>

            <Field>
              <FieldLabel>{{ t('automations.dialog.script.runFolder.label') }}</FieldLabel>
              <RadioGroup
                :model-value="dirMode === 'home' ? '' : dirMode"
                class="gap-1.5"
                data-testid="script-dialog-dirmode"
                @update:model-value="(v) => (dirMode = v === 'fixed' ? 'fixed' : 'kira')"
              >
                <div class="flex items-center gap-2">
                  <RadioGroupItem
                    id="script-dir-kira"
                    value="kira"
                    data-testid="script-dialog-dir-kira"
                  />
                  <Label for="script-dir-kira">{{ t('automations.dir.kira.label') }}</Label>
                </div>
                <div class="flex items-center gap-2">
                  <RadioGroupItem
                    id="script-dir-fixed"
                    value="fixed"
                    v-bind="form.invalidAttrs('folder')"
                    data-testid="script-dialog-dir-fixed"
                  />
                  <Label for="script-dir-fixed">{{
                    t('automations.dialog.script.chooseFolder.label')
                  }}</Label>
                </div>
              </RadioGroup>
              <div v-if="dirMode === 'fixed'" class="flex items-center gap-1.5">
                <span class="flex min-w-0 flex-1" data-testid="script-dialog-workingdir">
                  <PathChip v-if="workingDir" :path="workingDir" />
                  <span v-else class="truncate font-data">{{
                    t('automations.dialog.script.noFolder.status')
                  }}</span>
                </span>
                <Button
                  variant="dialog"
                  size="kira-lg"
                  data-testid="script-dialog-workingdir-choose"
                  @click="chooseWorkingDir"
                >
                  {{ t('common.action.choose.label') }}
                </Button>
              </div>
              <FieldDescription
                v-else-if="dirMode === 'home'"
                data-testid="script-dialog-home-notice"
              >
                <CatalogText
                  k="automations.dialog.script.homeNotice.description"
                  :params="{ home: '' }"
                >
                  <template #home><VarText :parts="homeParts" /></template>
                </CatalogText>
                <Button
                  variant="dialog"
                  size="kira-lg"
                  class="ml-2"
                  data-testid="script-dialog-use-kira"
                  @click="dirMode = 'kira'"
                >
                  {{ t('automations.dialog.script.useKira.label') }}
                </Button>
              </FieldDescription>
              <FieldDescription v-else data-testid="script-dialog-dir-preview">
                <VarText :parts="folderParts" />
              </FieldDescription>
              <FieldError v-if="blocker" data-testid="script-dialog-dir-blocker">{{
                blocker
              }}</FieldError>
              <FieldError
                v-if="errors.folder"
                :id="form.errorId('folder')"
                data-testid="script-dialog-folder-error"
                >{{
                  errors.folder
                }}</FieldError
              >
            </Field>

            <Field v-if="ade" orientation="horizontal">
              <Checkbox
                id="script-use-ade-dir"
                v-model="useAdeDir"
                data-testid="script-use-ade-dir"
              />
              <Label for="script-use-ade-dir">{{
                t('automations.dialog.script.useAdeDir.label')
              }}</Label>
            </Field>
          </TabsContent>

          <TabsContent
            value="params"
            force-mount
            class="flex flex-col gap-3 data-[state=inactive]:hidden"
          >
            <ParamsEditor
              v-model="params"
              v-model:secret-values="secretDraft"
              :kind="kind"
              :errors="errors.params"
              :saved-secrets="savedSecrets"
              :storage-off="storageOff"
            />
          </TabsContent>

          <TabsContent
            value="schedule"
            force-mount
            class="flex flex-col gap-3 data-[state=inactive]:hidden"
          >
            <Field orientation="horizontal">
              <Checkbox id="script-schedule" v-model="scheduleOn" data-testid="script-schedule" />
              <Label for="script-schedule">{{ t('automations.schedule.enable.label') }}</Label>
            </Field>
            <ScheduleFields
              v-if="schedule"
              v-model="schedule"
              @valid="(ok) => (scheduleOk = ok)"
              :script-id="script?.id ?? null"
              :kind="kind"
              :params="params"
              :saved-secrets="willBeSaved"
            />
            <FieldError v-if="errors.cron" data-testid="script-dialog-cron-error">{{
              errors.cron
            }}</FieldError>
          </TabsContent>

          <Alert v-if="error" variant="error" data-testid="script-dialog-error"
            ><AlertDescription>{{ error }}</AlertDescription></Alert
          >
        </DialogBody>
      </Tabs>

      <DialogFooter>
        <DialogClose as-child>
          <Button variant="dialog" size="kira-lg" data-testid="script-dialog-cancel">{{
            t('common.action.cancel.label')
          }}</Button>
        </DialogClose>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :pending="saving"
          data-testid="script-dialog-save"
          @click="save"
        >
          {{ script ? t('common.action.save.label') : t('common.action.add.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
