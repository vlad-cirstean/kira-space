<script setup lang="ts">
import { Alert } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Field, FieldDescription, FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@theme/components/ui/table';
import { type MessageKey, t } from '@theme/i18n';
import { formatDateTime } from '@workbench/behaviour/format';
import { announceApp } from '@workbench/behaviour/liveRegion';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { windowKey } from '@workbench/bridge/rpc';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref, useId } from 'vue';
import { control } from '../../bridge/control';
import { useConfigSyncMutations, useConfigSyncStatus } from '../../configsync/queries';
import type { SettingsPaneProps } from './types';

// Shared config: one shared Git repository pulled into the config folders. Own query over
// ConfigSyncService; the settings draft never holds any of this.
defineProps<SettingsPaneProps>();

const status = useConfigSyncStatus();
const m = useConfigSyncMutations();
const confirmDialogStore = useConfirmDialogStore();

const urlId = useId();
const branchId = useId();
const intervalId = useId();
const url = ref('');
const branch = ref('main');
const editing = ref(false);
const conflictHint = ref(false);
const pullError = ref<unknown>(null);

const data = computed(() => status.data.value);
const showForm = computed(() => !data.value?.configured || editing.value);
const failure = computed(
  () =>
    status.error.value ??
    m.setRepo.error.value ??
    m.fetchNow.error.value ??
    m.removeRepo.error.value ??
    m.setMode.error.value ??
    m.setFetchMinutes.error.value ??
    pullError.value,
);

const MODULE_LABEL: Record<string, MessageKey> = {
  connections: 'common.module.connections.label',
  api: 'space.settings.configSync.module.api.label',
  runs: 'space.settings.configSync.module.runs.label',
  workflows: 'space.settings.configSync.module.workflows.label',
  steps: 'space.settings.configSync.module.steps.label',
  agent: 'space.settings.configSync.module.agent.label',
  prompts: 'space.settings.configSync.module.prompts.label',
};
const MODE_LABEL: Record<string, MessageKey> = {
  local: 'space.settings.configSync.mode.local.option',
  shared: 'space.settings.configSync.mode.shared.option',
  combined: 'space.settings.configSync.mode.combined.option',
};

const fetchedText = computed(() => {
  const at = data.value?.fetchedAt;
  return at
    ? t('space.settings.configSync.fetched.status', { time: formatDateTime(at) })
    : t('space.settings.configSync.neverFetched.status');
});

function startChange(): void {
  url.value = data.value?.url ?? '';
  branch.value = data.value?.branch || 'main';
  editing.value = true;
}

async function onSave(): Promise<void> {
  await m.setRepo.mutateAsync({ url: url.value, branch: branch.value, windowKey });
  editing.value = false;
  url.value = '';
}

async function onRemove(): Promise<void> {
  const ok = await confirmDialogStore.confirm({
    title: t('space.settings.configSync.removeConfirm.title'),
    message: t('space.settings.configSync.removeConfirm.body'),
    confirmLabel: t('common.action.remove.label'),
    tone: 'danger',
  });
  if (ok) await m.removeRepo.mutateAsync();
}

function onInterval(e: Event): void {
  const minutes = Number((e.target as HTMLInputElement).value);
  if (Number.isInteger(minutes) && minutes > 0) m.setFetchMinutes.mutate(minutes);
}

function onReview(): void {
  fireAndForget(control.configSyncOpenReview(), 'configSync');
}

// A pull without conflicts applies in place; with conflicts the choices need the review window.
async function onPull(): Promise<void> {
  pullError.value = null;
  conflictHint.value = false;
  try {
    const preview = await control.configSyncPreview();
    if (preview.rows.some((r) => r.kind === 'conflict')) {
      conflictHint.value = true;
      await control.configSyncOpenReview();
      return;
    }
    await m.apply.mutateAsync({ token: preview.token, choices: {} });
    announceApp(t('space.settings.configSync.pulled.announce'));
  } catch (err) {
    pullError.value = err;
  }
}
</script>

<template>
  <div class="contents" v-show="active">
    <FieldDescription>{{ t('space.settings.configSync.description') }}</FieldDescription>

    <FieldError v-if="failure" data-testid="config-sync-pane-error">{{
      errorText(failure)
    }}</FieldError>

    <template v-if="showForm">
      <Field>
        <Label :for="urlId">{{ t('space.settings.configSync.url.label') }}</Label>
        <Input :id="urlId" v-model="url" data-testid="config-sync-url" />
      </Field>
      <Field>
        <Label :for="branchId">{{ t('common.field.branch.label') }}</Label>
        <Input :id="branchId" v-model="branch" data-testid="config-sync-branch" />
      </Field>
      <div>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="url.trim() === ''"
          :pending="m.setRepo.isPending.value"
          data-testid="config-sync-add"
          @click="onSave"
        >
          {{ t('space.settings.configSync.add.label') }}
        </Button>
      </div>
    </template>

    <template v-if="data?.configured && !editing">
      <div class="flex flex-col gap-1" data-testid="config-sync-repo">
        <span class="font-data break-all" data-testid="config-sync-repo-url">{{ data.url }}</span>
        <span class="font-data text-kira-sm text-muted-foreground">{{ data.branch }}</span>
        <span class="flex gap-3">
          <Button
            variant="link"
            size="inline"
            data-testid="config-sync-change"
            @click="startChange"
          >
            {{ t('space.settings.configSync.change.label') }}
          </Button>
          <Button variant="link" size="inline" data-testid="config-sync-remove" @click="onRemove">
            {{ t('common.action.remove.label') }}
          </Button>
        </span>
      </div>

      <Alert v-if="data.problem" variant="warn" data-testid="config-sync-problem">
        {{ data.problem.message }}
      </Alert>

      <Field>
        <Label :for="intervalId">{{ t('space.settings.configSync.interval.label') }}</Label>
        <Input
          :id="intervalId"
          type="number"
          min="1"
          :model-value="data.fetchMinutes"
          data-testid="config-sync-interval"
          @change="onInterval"
        />
      </Field>

      <div class="flex items-center gap-3">
        <Button
          variant="dialog"
          size="kira-lg"
          :pending="data.fetching || m.fetchNow.isPending.value"
          data-testid="config-sync-fetch"
          @click="m.fetchNow.mutate()"
        >
          {{ t('space.settings.configSync.fetchNow.label') }}
        </Button>
        <span class="text-kira-sm text-muted-foreground" data-testid="config-sync-fetched">{{
          fetchedText
        }}</span>
      </div>

      <div class="flex items-center gap-3" data-testid="config-sync-changes">
        <span data-testid="config-sync-changes-status">{{
          data.changes > 0
            ? t('space.settings.configSync.changes.status', { count: data.changes })
            : t('space.settings.configSync.upToDate.status')
        }}</span>
        <template v-if="data.changes > 0">
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="config-sync-review"
            @click="onReview"
          >
            {{ t('space.configSync.toast.review.label') }}
          </Button>
          <Button
            variant="dialog-primary"
            size="kira-lg"
            :pending="m.apply.isPending.value"
            data-testid="config-sync-pull"
            @click="onPull"
          >
            {{ t('git.toolbar.pull.label') }}
          </Button>
        </template>
      </div>
      <FieldDescription v-if="conflictHint" data-testid="config-sync-conflict-hint">{{
        t('space.settings.configSync.pullConflicts.hint')
      }}</FieldDescription>

      <Table v-if="data.modules.length > 0" data-testid="config-sync-modules">
        <TableHeader>
          <TableRow class="hover:bg-transparent">
            <TableHead>{{ t('space.settings.configSync.module.label') }}</TableHead>
            <TableHead>{{ t('common.mode.label') }}</TableHead>
            <TableHead>{{ t('space.settings.configSync.units.label') }}</TableHead>
            <TableHead>{{ t('space.settings.configSync.deactivated.label') }}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow
            v-for="mod in data.modules"
            :key="mod.name"
            :data-testid="`config-sync-module-${mod.name}`"
          >
            <TableCell>{{
              MODULE_LABEL[mod.name] ? t(MODULE_LABEL[mod.name]) : mod.name
            }}</TableCell>
            <TableCell>
              <NativeSelect
                variant="bordered"
                :model-value="mod.mode"
                :data-testid="`config-sync-mode-${mod.name}`"
                @update:model-value="(v) => m.setMode.mutate({ module: mod.name, mode: String(v) })"
              >
                <option v-for="mode in mod.modes" :key="mode" :value="mode">
                  {{ MODE_LABEL[mode] ? t(MODE_LABEL[mode]) : mode }}
                </option>
              </NativeSelect>
            </TableCell>
            <TableCell>{{ mod.units }}</TableCell>
            <TableCell>{{ mod.deactivated }}</TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </template>
  </div>
</template>
