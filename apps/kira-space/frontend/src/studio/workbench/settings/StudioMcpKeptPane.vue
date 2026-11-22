<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import ResetToDefaultButton from '@theme/components/ResetToDefaultButton.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
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
import { keptDetails } from '../../kept/keptFormat';
import { useClearKept, useKeptVars } from '../../kept/keptQueries';
import TempVarRow from '../../kept/TempVarRow.vue';

// Temporary variables live in the backend's memory: list, read in full, delete. The lifetime applies to
// values set from now on. The agent never sees a value; this view is where the user does.
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
    <FieldSet class="min-w-0">
      <div class="flex items-center justify-between gap-1">
        <FieldLegend>{{ t('common.temporaryVariables.label') }}</FieldLegend>
        <Button
          v-if="entries.length"
          variant="link-meta"
          size="inline"
          :pending="clear.isPending.value"
          data-testid="settings-kept-clear-all"
          @click="clearAll"
        >
          {{ t('common.action.clearAll.label') }}
        </Button>
      </div>
      <FieldDescription>{{ t('studio.settings.kept.intro.description') }}</FieldDescription>
      <QueryState
        :query="list"
        variant="inline"
        :is-empty="() => false"
        error-testid="settings-kept-error"
      >
        <Card v-if="entries.length" variant="list">
          <ul class="m-0 flex list-none flex-col p-0" data-testid="settings-kept-list">
            <li
              v-for="e in entries"
              :key="e.id"
              :class="
                cn(rowVariants({ layout: 'double' }), 'min-w-0 items-center whitespace-normal')
              "
              data-testid="settings-kept-entry"
            >
              <TempVarRow
                class="flex-1"
                :name="e.name"
                :secret="e.secret"
                :entry="e"
                :expires-at="e.expiresAt"
                :details="keptDetails(e, { origin: e.origin })"
              >
                <template #actions>
                  <TooltipIconButton
                    :icon="ICON.delete"
                    :label="t('studio.settings.kept.clear.aria', { name: e.name })"
                    data-testid="kept-entry-clear"
                    @click="clearOne.run([e.id])"
                  />
                </template>
              </TempVarRow>
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
      <Field orientation="horizontal" class="justify-between">
        <FieldContent>
          <FieldLabel :for="retentionId">{{
            t('studio.settings.kept.retention.label')
          }}</FieldLabel>
          <FieldDescription>{{ t('studio.settings.kept.retention.description') }}</FieldDescription>
        </FieldContent>
        <div class="flex shrink-0 items-center gap-1">
          <NumberStepperInput
            :id="retentionId"
            class="w-24"
            :min="KEPT_RETENTION_MINUTES_RANGE.min"
            :max="KEPT_RETENTION_MINUTES_RANGE.max"
            :model-value="draft"
            data-testid="settings-kept-retention"
            @input="onRetention"
          />
          <ResetToDefaultButton
            data-testid="settings-reset-dbMcp-keptRetentionMinutes"
            :at-default="minutes === KEPT_RETENTION_MINUTES_DEFAULT"
            @click="retention.run(KEPT_RETENTION_MINUTES_DEFAULT)"
          />
        </div>
      </Field>
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
    </FieldSet>
  </div>
</template>
