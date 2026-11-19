<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import ResetToDefaultButton from '@theme/components/ResetToDefaultButton.vue';
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
import { useAsyncAction, useMutationAction } from '@workbench/behaviour/asyncAction';
import QueryState from '@workbench/components/QueryState.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref, useId, watch } from 'vue';
import { useSettingsStore } from '../../../state/settings';
import {
  KEPT_RETENTION_MINUTES_DEFAULT,
  KEPT_RETENTION_MINUTES_RANGE,
} from '../../../state/settingsDomain';
import { useClearKept, useKeptVars } from '../../kept/keptQueries';
import KeptEntryRow from './KeptEntryRow.vue';

// Kept variables live in the backend's memory: list, read in full, clear. Retention applies to values
// kept from now on. The agent never sees a value; this view is where the user does.
defineProps<{ active: boolean }>();

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
  draftNumber.value >= KEPT_RETENTION_MINUTES_RANGE.min &&
  draftNumber.value <= KEPT_RETENTION_MINUTES_RANGE.max
    ? ''
    : t('studio.settings.kept.retention.error', {
        min: KEPT_RETENTION_MINUTES_RANGE.min,
        max: KEPT_RETENTION_MINUTES_RANGE.max,
      }),
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

const clearOne = useMutationAction(clear);
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
      <div class="flex items-center justify-between gap-1">
        <FieldLabel :for="retentionId">{{ t('studio.settings.kept.retention.label') }}</FieldLabel>
        <ResetToDefaultButton
          data-testid="settings-reset-dbMcp-keptRetentionMinutes"
          :at-default="minutes === KEPT_RETENTION_MINUTES_DEFAULT"
          @click="retention.run(KEPT_RETENTION_MINUTES_DEFAULT)"
        />
      </div>
      <NumberStepperInput
        :id="retentionId"
        :min="KEPT_RETENTION_MINUTES_RANGE.min"
        :max="KEPT_RETENTION_MINUTES_RANGE.max"
        :model-value="draft"
        data-testid="settings-kept-retention"
        @input="onRetention"
      />
      <FieldError v-if="retentionError" data-testid="settings-kept-retention-error">{{
        retentionError
      }}</FieldError>
      <FieldDescription v-else>{{
        t('studio.settings.kept.retention.description')
      }}</FieldDescription>
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
      <QueryState
        :query="list"
        variant="inline"
        :is-empty="() => false"
        error-testid="settings-kept-error"
      >
        <Card v-if="entries.length" variant="list">
          <ul class="m-0 flex list-none flex-col p-0" data-testid="settings-kept-list">
            <li v-for="e in entries" :key="e.id" data-testid="settings-kept-entry">
              <KeptEntryRow :entry="e" @clear="clearOne.run([e.id])" />
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
