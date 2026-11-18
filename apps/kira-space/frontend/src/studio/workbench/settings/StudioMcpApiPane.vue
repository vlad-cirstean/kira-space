<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Field, FieldContent, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed, useId } from 'vue';
import { useSettingsStore } from '../../../state/settings';
import type { SettingsPaneProps } from '../../../workbench/settings/types';
import { useApiAgentAccess } from '../../api/state/apiConfig';
import ApiAccessRulesSection from './ApiAccessRulesSection.vue';

// Studio MCP settings: API request tools (switch, approval mode, access rules). Instant actions
// bypass draft/Save; a rejected call reports here so a snapped-back switch is not the only trace.
defineProps<SettingsPaneProps>();

const settingsStore = useSettingsStore();
const apiRequests = useAsyncAction((enabled: boolean) =>
  settingsStore.patchSettings({ dbMcp: { apiRequests: enabled } }),
);
const apiApproval = useAsyncAction((mode: 'always' | 'never') =>
  settingsStore.patchSettings({ dbMcp: { apiApproval: mode } }),
);
const actionError = computed(() => apiRequests.error.value ?? apiApproval.error.value);
function clearActionErrors(): void {
  apiRequests.error.value = null;
  apiApproval.error.value = null;
}
async function onToggleApiRequests(enabled: boolean): Promise<void> {
  clearActionErrors();
  await apiRequests.run(enabled);
}
async function onApiApprovalChange(value: unknown): Promise<void> {
  clearActionErrors();
  await apiApproval.run(value === 'never' ? 'never' : 'always');
}
const agentAccess = useApiAgentAccess();
const sharedVariableCount = computed(() =>
  (agentAccess.data.value ?? []).reduce((n, env) => n + env.variables.length, 0),
);

// `for`/`id` keeps the label-to-control association now that the field wrapper is a plain <Field> div.
const dbMcpApiId = useId();
const approvalItems = computed(() => [
  {
    value: 'always',
    label: t('studio.settings.dbMcp.apiApproval.always.option'),
    disabled: apiApproval.pending.value,
    testid: 'settings-db-mcp-api-approval-always',
  },
  {
    value: 'never',
    label: t('studio.settings.dbMcp.apiApproval.never.option'),
    disabled: apiApproval.pending.value,
    testid: 'settings-db-mcp-api-approval-never',
  },
]);
</script>

<template>
  <div class="contents" v-show="active">
    <Field orientation="horizontal">
      <Switch
        :id="dbMcpApiId"
        :model-value="settingsStore.dbMcp.apiRequests"
        :disabled="apiRequests.pending.value"
        data-testid="settings-db-mcp-api"
        @update:model-value="(v) => onToggleApiRequests(v === true)"
      />
      <FieldContent>
        <FieldLabel :for="dbMcpApiId">{{ t('studio.settings.dbMcp.api.label') }}</FieldLabel>
        <FieldDescription>{{ t('studio.settings.dbMcp.api.description') }}</FieldDescription>
        <FieldDescription data-testid="settings-db-mcp-api-count">{{
          t('studio.settings.dbMcp.api.count.status', {
            count: sharedVariableCount,
            environments: (agentAccess.data.value ?? []).length,
          })
        }}</FieldDescription>
      </FieldContent>
    </Field>
    <Field v-if="settingsStore.dbMcp.apiRequests">
      <FieldLabel>{{ t('studio.settings.dbMcp.apiApproval.label') }}</FieldLabel>
      <SecondaryTabs
        variant="choice"
        size="kira-lg"
        :aria-label="t('studio.settings.dbMcp.apiApproval.label')"
        :model-value="settingsStore.dbMcp.apiApproval"
        :items="approvalItems"
        data-testid="settings-db-mcp-api-approval"
        @update:model-value="onApiApprovalChange"
      />
      <Alert
        v-if="settingsStore.dbMcp.apiApproval === 'never'"
        variant="warn"
        data-testid="settings-db-mcp-api-never-warning"
      >
        <AlertDescription>{{ t('studio.settings.dbMcp.apiApproval.never.body') }}</AlertDescription>
      </Alert>
    </Field>
    <ApiAccessRulesSection v-if="settingsStore.dbMcp.apiRequests" />

    <Alert v-if="actionError" variant="error" data-testid="settings-db-mcp-api-error">
      <AlertDescription>{{ actionError }}</AlertDescription>
    </Alert>
  </div>
</template>
