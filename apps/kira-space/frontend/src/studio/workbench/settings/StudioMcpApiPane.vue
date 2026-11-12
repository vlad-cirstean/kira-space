<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Field, FieldContent, FieldDescription } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
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
const dbMcpApiApprovalId = useId();
</script>

<template>
  <div class="contents" v-show="active">
    <Field orientation="horizontal" class="items-start">
      <Switch
        :id="dbMcpApiId"
        :model-value="settingsStore.dbMcp.apiRequests"
        :disabled="apiRequests.pending.value"
        data-testid="settings-db-mcp-api"
        @update:model-value="(v) => onToggleApiRequests(v === true)"
      />
      <FieldContent>
        <Label :for="dbMcpApiId">{{ t('studio.settings.dbMcp.api.label') }}</Label>
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
      <Label :for="dbMcpApiApprovalId">{{ t('studio.settings.dbMcp.apiApproval.label') }}</Label>
      <NativeSelect
        :id="dbMcpApiApprovalId"
        variant="bordered"
        size="kira-lg"
        class="self-start"
        data-testid="settings-db-mcp-api-approval"
        :model-value="settingsStore.dbMcp.apiApproval"
        :disabled="apiApproval.pending.value"
        @update:model-value="onApiApprovalChange"
      >
        <option value="always">{{ t('studio.settings.dbMcp.apiApproval.always.option') }}</option>
        <option value="never">{{ t('studio.settings.dbMcp.apiApproval.never.option') }}</option>
      </NativeSelect>
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
