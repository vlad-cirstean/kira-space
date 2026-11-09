<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { FieldDescription, FieldLegend } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed, ref } from 'vue';
import { useSettingsStore } from '../../../state/settings';
import type { ApiRule } from '../../../state/settingsDomain';
import ApiAccessRuleDialog from '../apiaccess/ApiAccessRuleDialog.vue';
import { allowsNothing } from '../apiaccess/apiAccessRules';

// The user's host and method rules for the api_request tool, under the approval mode. Each save or
// remove sends the whole new list in one settings write, like the other instant settings here.
const settingsStore = useSettingsStore();
const rules = computed(() => settingsStore.dbMcp.apiRules);

const dialogOpen = ref(false);
const editing = ref<number | null>(null);
const editingRule = computed(() =>
  editing.value === null ? undefined : rules.value[editing.value],
);

const write = useAsyncAction((next: ApiRule[]) =>
  settingsStore.patchSettings({ dbMcp: { apiRules: next } }),
);
const remove = useAsyncAction((next: ApiRule[]) =>
  settingsStore.patchSettings({ dbMcp: { apiRules: next } }),
);

function openDialog(index: number | null): void {
  editing.value = index;
  write.error.value = null;
  dialogOpen.value = true;
}

async function onSave(rule: ApiRule): Promise<void> {
  const next = [...rules.value];
  if (editing.value === null) next.push(rule);
  else next[editing.value] = rule;
  if (await write.run(next)) dialogOpen.value = false;
}

function onRemove(index: number): void {
  remove.run(rules.value.filter((_, i) => i !== index));
}
</script>

<template>
  <FieldLegend>{{ t('studio.settings.dbMcp.apiRules.legend.label') }}</FieldLegend>
  <FieldDescription>{{ t('studio.settings.dbMcp.apiRules.description') }}</FieldDescription>
  <Alert
    v-if="settingsStore.dbMcp.apiRulesInvalid"
    variant="error"
    data-testid="settings-db-mcp-api-rules-invalid"
  >
    <AlertDescription>{{ t('studio.settings.dbMcp.apiRules.invalid.body') }}</AlertDescription>
  </Alert>
  <ul
    v-if="rules.length"
    class="flex flex-col m-0 p-0 list-none gap-0.5"
    data-testid="settings-db-mcp-api-rules"
  >
    <li v-for="(rule, i) in rules" :key="i">
      <Card
        variant="list"
        class="flex-row items-center justify-between gap-1.5"
        data-testid="settings-db-mcp-api-rule"
      >
        <div class="flex min-w-0 items-center gap-1.5">
          <Badge :variant="rule.effect === 'allow' ? 'ok' : 'error'">{{
            t(
              rule.effect === 'allow'
                ? 'studio.project.connectionDialog.mcp.allow.label'
                : 'common.action.deny.label',
            )
          }}</Badge>
          <span class="break-all font-data text-kira-md">{{ rule.hosts.join(', ') }}</span>
          <span class="shrink-0 text-muted-foreground text-kira-md">{{
            rule.methods.includes('*')
              ? t('studio.settings.dbMcp.apiRules.anyMethod.label')
              : rule.methods.join(', ')
          }}</span>
        </div>
        <div class="flex shrink-0 items-center gap-0.5">
          <Button
            variant="toolbar"
            size="kira-icon"
            :aria-label="t('studio.settings.dbMcp.apiRules.edit.aria')"
            data-testid="api-rule-edit"
            @click="openDialog(i)"
          >
            <CodiconIcon :name="ICON.edit" :size="13" />
          </Button>
          <Button
            variant="toolbar"
            size="kira-icon"
            :pending="remove.pending.value"
            :aria-label="t('studio.project.connectionDialog.removeRule.tooltip')"
            data-testid="api-rule-remove"
            @click="onRemove(i)"
          >
            <CodiconIcon :name="ICON.delete" :size="13" />
          </Button>
        </div>
      </Card>
    </li>
  </ul>
  <FieldDescription v-else data-testid="settings-db-mcp-api-rules-empty">{{
    t('studio.settings.dbMcp.apiRules.empty.body')
  }}</FieldDescription>
  <Alert
    v-if="allowsNothing(rules)"
    variant="warn"
    data-testid="settings-db-mcp-api-rules-no-allow"
  >
    <AlertDescription>{{ t('studio.settings.dbMcp.apiRules.noAllow.body') }}</AlertDescription>
  </Alert>
  <Alert v-if="remove.error.value" variant="error" data-testid="settings-db-mcp-api-rules-error">
    <AlertDescription>{{ remove.error.value }}</AlertDescription>
  </Alert>
  <Button
    variant="dialog"
    size="kira-lg"
    class="self-start"
    data-testid="settings-db-mcp-api-rules-add"
    @click="openDialog(null)"
  >
    <CodiconIcon :name="ICON.add" :size="13" />
    {{ t('studio.settings.dbMcp.apiRules.add.label') }}
  </Button>
  <ApiAccessRuleDialog
    v-model:open="dialogOpen"
    :rule="editingRule"
    :pending="write.pending.value"
    :error="write.error.value"
    @save="onSave"
  />
</template>
