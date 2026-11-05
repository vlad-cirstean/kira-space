<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Card } from '@theme/components/ui/card';
import { FieldDescription, FieldLegend } from '@theme/components/ui/field';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { applyRunFileEdits } from '@workbench/automations/runFileQueries';
import { runFileName } from '@workbench/automations/scriptActions';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed } from 'vue';
import { useCustomScriptsStore } from '../../state/customScripts';

// Which normal scripts an agent on the Database MCP server may list and run. The switch edits the
// script's run file (the mcp block); description and approval mode are edited in its MCP tab.
const store = useCustomScriptsStore();
const normal = computed(() => store.records.filter((s) => s.kind !== 'smart'));

const toggle = useAsyncAction((id: string, enabled: boolean) =>
  applyRunFileEdits(store, runFileName(id), [
    { path: ['mcp', 'enabled'], value: enabled, unset: false },
  ]),
);
</script>

<template>
  <FieldLegend>{{ t('studio.settings.dbMcp.scripts.legend.label') }}</FieldLegend>
  <FieldDescription>{{ t('studio.settings.dbMcp.scripts.intro.description') }}</FieldDescription>
  <ul
    v-if="normal.length"
    class="flex flex-col m-0 p-0 list-none gap-0.5"
    data-testid="db-mcp-scripts-list"
  >
    <li v-for="s in normal" :key="s.id">
      <Card
        variant="list"
        class="flex-row items-center justify-between gap-1.5"
        :data-testid="`db-mcp-script-row-${s.id}`"
      >
        <div class="flex flex-col min-w-0 gap-0.5">
          <span class="break-words text-fg text-kira-md">{{ s.name }}</span>
          <FieldDescription v-if="s.mcp?.enabled">{{
            t(
              s.mcp.run === 'allow'
                ? 'studio.settings.dbMcp.scripts.allow.status'
                : 'studio.settings.dbMcp.scripts.prompt.status',
            )
          }}</FieldDescription>
        </div>
        <div class="flex shrink-0 items-center gap-1.5">
          <Badge
            :variant="s.mcp?.enabled ? 'ok' : 'default'"
            :data-testid="`db-mcp-script-${s.id}`"
            >{{
              s.mcp?.enabled ? t('studio.settings.dbMcp.exposed.status') : t('common.off.status')
            }}</Badge
          >
          <Switch
            :model-value="s.mcp?.enabled === true"
            :disabled="toggle.pending.value"
            :aria-label="t('studio.settings.dbMcp.scripts.toggle.aria', { name: s.name })"
            :data-testid="`db-mcp-script-toggle-${s.id}`"
            @update:model-value="(v) => toggle.run(s.id, v === true)"
          />
        </div>
      </Card>
    </li>
  </ul>
  <FieldDescription v-else data-testid="db-mcp-scripts-empty">
    {{ t('automations.panel.none.empty') }}
  </FieldDescription>
  <Alert v-if="toggle.error.value" variant="error" data-testid="db-mcp-scripts-error">
    <AlertDescription>{{ toggle.error.value }}</AlertDescription>
  </Alert>
</template>
