<script setup lang="ts">
import type { KeptEntry } from '@shared/domain/kept';
import InlineState from '@theme/components/InlineState.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import QueryState from '@workbench/components/QueryState.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref, useId, watch } from 'vue';
import { useSettingsStore } from '../../../state/settings';
import { useClearKept, useKeptVars } from '../../kept/keptQueries';
import KeptEntryRow from './KeptEntryRow.vue';

// Kept variables live in the backend's memory: list, read in full, clear. Retention applies to values
// kept from now on. The agent never sees a value; this view is where the user does.
defineProps<{ active: boolean }>();

const RETENTION_MIN = 5;
const RETENTION_MAX = 240;

const settingsStore = useSettingsStore();
const list = useKeptVars();
const clear = useClearKept();
const confirmStore = useConfirmDialogStore();

const entries = computed(() =>
  [...(list.data.value?.entries ?? [])].sort(
    (a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt),
  ),
);
const minutes = computed(() => settingsStore.dbMcp.keptRetentionMinutes);

const retentionId = useId();
const draft = ref(String(minutes.value));
watch(minutes, (m) => (draft.value = String(m)));
const draftNumber = computed(() => (draft.value.trim() === '' ? Number.NaN : Number(draft.value)));
const retentionError = computed(() =>
  Number.isInteger(draftNumber.value) &&
  draftNumber.value >= RETENTION_MIN &&
  draftNumber.value <= RETENTION_MAX
    ? ''
    : t('studio.settings.kept.retention.error', { min: RETENTION_MIN, max: RETENTION_MAX }),
);
const retention = useAsyncAction((value: number) =>
  settingsStore.patchSettings({ dbMcp: { keptRetentionMinutes: value } }),
);
async function onRetention(e: Event): Promise<void> {
  draft.value = (e.target as HTMLInputElement).value;
  if (retentionError.value === '' && draftNumber.value !== minutes.value) {
    await retention.run(draftNumber.value);
  }
}

const clearOne = useAsyncAction((e: KeptEntry) => clear.mutateAsync([e.id]));
function clearAll(): void {
  void confirmStore.confirm({
    title: t('studio.settings.kept.clearAll.title'),
    message: t('studio.settings.kept.clearAll.body'),
    confirmLabel: t('common.action.clearAll.label'),
    tone: 'danger',
    run: () => clear.mutateAsync('all'),
  });
}
</script>

<template>
  <div class="contents" v-show="active">
    <Field>
      <FieldLabel :for="retentionId">{{ t('studio.settings.kept.retention.label') }}</FieldLabel>
      <NumberStepperInput
        :id="retentionId"
        :min="RETENTION_MIN"
        :max="RETENTION_MAX"
        :model-value="draft"
        data-testid="settings-kept-retention"
        @input="onRetention"
      />
      <FieldDescription>{{ t('studio.settings.kept.retention.description') }}</FieldDescription>
      <FieldError v-if="retentionError" data-testid="settings-kept-retention-error">{{
        retentionError
      }}</FieldError>
      <Alert
        v-if="retention.error.value"
        variant="error"
        data-testid="settings-kept-retention-fail"
      >
        <AlertDescription>{{ retention.error.value }}</AlertDescription>
      </Alert>
    </Field>

    <FieldSet>
      <FieldLegend>{{ t('studio.settings.kept.legend.label') }}</FieldLegend>
      <div v-if="entries.length" class="flex justify-end">
        <Button
          variant="dialog"
          size="kira-lg"
          :pending="clear.isPending.value"
          data-testid="settings-kept-clear-all"
          @click="clearAll"
        >
          {{ t('common.action.clearAll.label') }}
        </Button>
      </div>
      <QueryState :query="list" variant="inline" :is-empty="() => false">
        <template #error="{ message, retry }">
          <Alert variant="error" data-testid="settings-kept-error">
            <AlertDescription>{{ message }}</AlertDescription>
          </Alert>
          <Button
            variant="dialog"
            size="kira-lg"
            class="self-start"
            data-testid="settings-kept-retry"
            @click="retry"
          >
            {{ t('common.action.retry.label') }}
          </Button>
        </template>
        <Card v-if="entries.length" variant="list">
          <ul class="m-0 flex list-none flex-col p-0" data-testid="settings-kept-list">
            <li v-for="e in entries" :key="e.id" data-testid="settings-kept-entry">
              <KeptEntryRow :entry="e" @clear="clearOne.run(e)" />
            </li>
          </ul>
        </Card>
        <InlineState v-else kind="empty" data-testid="settings-kept-empty">{{
          t('studio.settings.kept.empty.empty', {
            minutes: list.data.value?.retentionMinutes ?? minutes,
          })
        }}</InlineState>
      </QueryState>
      <Alert v-if="clearOne.error.value" variant="error" data-testid="settings-kept-clear-error">
        <AlertDescription>{{ clearOne.error.value }}</AlertDescription>
      </Alert>
    </FieldSet>
  </div>
</template>
