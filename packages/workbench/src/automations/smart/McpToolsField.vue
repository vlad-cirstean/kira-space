<script setup lang="ts">
import type { ScriptMcpServer, SmartSettings } from '@shared/domain/scripts';
import { useQuery } from '@tanstack/vue-query';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Checkbox } from '@theme/components/ui/checkbox';
import { FieldDescription, FieldLegend, FieldSet } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import QueryState from '@workbench/components/QueryState.vue';
import { computed, ref, useId } from 'vue';
import { useAutomationsModule } from '../module';
import McpServerTools from './McpServerTools.vue';
import { withMcp } from './smartSettings';

const smart = defineModel<SmartSettings>({ required: true });
const { runs } = useAutomationsModule();
const idBase = useId();

const servers = useQuery({
  queryKey: ['scriptRuns', 'mcp'],
  queryFn: () => runs.mcpServers(),
  staleTime: 0,
});

function chosen(server: string): string[] {
  return smart.value.mcp.find((c) => c.server === server)?.tools ?? [];
}

function allTools(server: string): boolean {
  return smart.value.mcp.find((c) => c.server === server)?.allTools ?? false;
}

function setServer(server: string, on: boolean): void {
  smart.value.mcp = on
    ? withMcp(smart.value.mcp, server, [], true)
    : smart.value.mcp.filter((c) => c.server !== server);
}

function setAllTools(server: string, on: boolean): void {
  smart.value.mcp = withMcp(smart.value.mcp, server, chosen(server), on);
}

function setTool(server: string, tool: string, on: boolean): void {
  const cur = chosen(server);
  const next = on ? [...cur.filter((x) => x !== tool), tool] : cur.filter((x) => x !== tool);
  smart.value.mcp = withMcp(smart.value.mcp, server, next, false);
}

const typed = ref<Record<string, string>>({});
function addByName(server: string): void {
  const name = (typed.value[server] ?? '').trim();
  if (name === '') return;
  setTool(server, name, true);
  typed.value = { ...typed.value, [server]: '' };
}

function statusText(status: ScriptMcpServer['status']): string {
  switch (status) {
    case 'connected':
      return t('automations.smart.mcpStatusConnected.status');
    case 'failed':
      return t('automations.smart.mcpStatusFailed.status');
    case 'needsAuth':
      return t('automations.smart.mcpStatusNeedsAuth.status');
    case 'pending':
      return t('automations.smart.mcpStatusPending.status');
    default:
      return t('automations.smart.mcpStatusUnknown.status');
  }
}

const enabled = computed(() => smart.value.mcp.map((c) => c.server));
</script>

<template>
  <FieldSet data-testid="smart-mcp">
    <FieldLegend>{{ t('automations.smart.mcp.label') }}</FieldLegend>
    <FieldDescription>{{
      servers.data.value?.source === 'app'
        ? t('automations.smart.mcpHintApp.hint')
        : t('automations.smart.mcpHint.hint')
    }}</FieldDescription>
    <QueryState :query="servers" variant="inline" :is-empty="(d) => d.servers.length === 0">
      <template #loading />
      <template #empty>
        <span class="text-kira-sm text-muted-foreground" data-testid="smart-mcp-none">{{
          t('automations.smart.mcpNone.emptyHint')
        }}</span>
      </template>
      <template #error="{ message }">
        <Alert variant="error">
          <AlertDescription>{{ message }}</AlertDescription>
        </Alert>
      </template>
      <template #default="{ data }">
        <div
          v-for="s in data.servers"
          :key="s.name"
          class="flex flex-col gap-1"
          data-testid="smart-mcp-server"
          :data-server="s.name"
        >
          <div class="flex items-center gap-2">
            <Checkbox
              :id="`${idBase}-${s.name}`"
              :model-value="enabled.includes(s.name)"
              :data-testid="`smart-mcp-switch-${s.name}`"
              @update:model-value="(v) => setServer(s.name, v === true)"
            />
            <Label :for="`${idBase}-${s.name}`">{{ s.name }}</Label>
            <span class="truncate font-data text-kira-sm text-muted-foreground"
              >{{ s.transport }} {{ s.target }}</span
            >
            <span
              v-if="s.status !== ''"
              class="text-kira-sm text-muted-foreground"
              data-testid="smart-mcp-status"
              >{{
                statusText(s.status)
              }}</span
            >
          </div>
          <McpServerTools
            v-if="enabled.includes(s.name)"
            :server="s.name"
            :all-tools="allTools(s.name)"
            :chosen="chosen(s.name)"
            :typed="typed[s.name] ?? ''"
            @all-tools="(on) => setAllTools(s.name, on)"
            @tool="(tool, on) => setTool(s.name, tool, on)"
            @typed="(v) => (typed = { ...typed, [s.name]: v })"
            @add="addByName(s.name)"
          />
        </div>
      </template>
    </QueryState>
  </FieldSet>
</template>
