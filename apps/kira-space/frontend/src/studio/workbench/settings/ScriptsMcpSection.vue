<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { FieldDescription, FieldLegend } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { applyRunFileEdits } from '@workbench/automations/runFileQueries';
import { runFileName } from '@workbench/automations/scriptActions';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed } from 'vue';
import { useCustomScriptsStore } from '../../../state/customScripts';
import McpExposureRow from './McpExposureRow.vue';

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
      <McpExposureRow
        :id="s.id"
        testid="db-mcp-script"
        :name="s.name"
        :enabled="s.mcp?.enabled === true"
        :status="
          t(
            s.mcp?.run === 'allow'
              ? 'studio.settings.dbMcp.scripts.allow.status'
              : 'studio.settings.dbMcp.scripts.prompt.status',
          )
        "
        :pending="toggle.pending.value"
        :toggle-aria="t('studio.settings.dbMcp.scripts.toggle.aria', { name: s.name })"
        @toggle="(on) => toggle.run(s.id, on)"
      >
        <Badge
          v-if="s.mcp?.enabled && s.mcp.mode === 'long'"
          variant="default"
          :data-testid="`db-mcp-script-long-${s.id}`"
          >{{
            t('studio.settings.dbMcp.scripts.long.status')
          }}</Badge
        >
      </McpExposureRow>
    </li>
  </ul>
  <FieldDescription v-else data-testid="db-mcp-scripts-empty">
    {{ t('automations.panel.none.empty') }}
  </FieldDescription>
  <Alert v-if="toggle.error.value" variant="error" data-testid="db-mcp-scripts-error">
    <AlertDescription>{{ toggle.error.value }}</AlertDescription>
  </Alert>
</template>
