<script setup lang="ts">
import type { ConnectionSummary } from '@shared/domain/connection';
import { useQuery } from '@tanstack/vue-query';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { FieldDescription, FieldLegend } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import type { SettingsPaneProps } from '../../../workbench/settings/types';
import { useConnectionDialogStore } from '../../state/connectionDialogs';
import { useConnectionsStore } from '../../state/connections';
import { loadMaskRuleCounts, maskRuleCountsQueryKey } from '../../state/maskRules';

// Studio MCP settings: which connections the server exposes. Exposure and the permission modes live
// in the connection's file; this list is a glance plus the way into the file.
defineProps<SettingsPaneProps>();

const connectionsStore = useConnectionsStore();
const connectionDialogStore = useConnectionDialogStore();
const maskRuleCountsQuery = useQuery({
  queryKey: maskRuleCountsQueryKey,
  queryFn: loadMaskRuleCounts,
});

/** Status line of a connection's result review; empty while every mode follows the app setting. */
function reviewStatus(conn: ConnectionSummary): string {
  const { read, write, ddl } = conn.mcpReview;
  if (read === 'inherit' && write === 'inherit' && ddl === 'inherit') return '';
  if (read === 'review' && write === 'review' && ddl === 'review')
    return t('studio.settings.dbMcp.reviewAlways.status');
  if (read === 'send' && write === 'send' && ddl === 'send')
    return t('studio.settings.dbMcp.reviewNever.status');
  return t('studio.settings.dbMcp.reviewMixed.status');
}

// The row's own description glance: first line only, "" when unset.
function mcpDescriptionFirstLine(conn: ConnectionSummary): string {
  return conn.mcpDescription.split('\n', 1)[0] ?? '';
}
</script>

<template>
  <div class="contents" v-show="active">
    <FieldLegend>{{ t('studio.settings.dbMcp.exposed.label') }}</FieldLegend>
    <FieldDescription>{{ t('studio.settings.dbMcp.exposed.description') }}</FieldDescription>
    <FieldDescription>{{ t('studio.settings.dbMcp.defaults.hint') }}</FieldDescription>
    <ul
      v-if="connectionsStore.records.length"
      class="flex flex-col m-0 p-0 list-none gap-0.5"
      data-testid="db-mcp-connections-list"
    >
      <li v-for="conn in connectionsStore.records" :key="conn.id">
        <Card
          variant="list"
          class="flex-row items-center justify-between gap-1.5"
          :data-testid="`db-mcp-connection-row-${conn.id}`"
        >
          <div class="flex flex-col min-w-0 gap-0.5">
            <span class="break-words text-fg text-kira-md">{{ conn.name }}</span>
            <FieldDescription>{{
              t(
                conn.mcpAutoExplain
                  ? 'studio.settings.dbMcp.modesPlans.status'
                  : 'studio.settings.dbMcp.modes.status',
                {
                  read: conn.mcpReadMode,
                  write: conn.mcpWriteMode,
                  ddl: conn.mcpDdlMode,
                },
              )
            }}</FieldDescription>
            <FieldDescription
              v-if="reviewStatus(conn)"
              :data-testid="`db-mcp-connection-review-${conn.id}`"
              >{{
                reviewStatus(conn)
              }}</FieldDescription
            >
            <FieldDescription v-if="mcpDescriptionFirstLine(conn)">{{
              mcpDescriptionFirstLine(conn)
            }}</FieldDescription>
            <!-- M5 §7.5: extends this existing read-only glance — no second full editor
               here (M2's own established split); editing lives in the connection's own
               Privacy tab. -->
            <FieldDescription
              v-if="maskRuleCountsQuery.data.value?.[conn.id]"
              :data-testid="`db-mcp-connection-masked-${conn.id}`"
              >{{
                t('studio.settings.dbMcp.masked.status', {
                  count: maskRuleCountsQuery.data.value?.[conn.id] ?? 0,
                })
              }}</FieldDescription
            >
          </div>
          <div class="flex shrink-0 items-center gap-1.5">
            <Badge
              :variant="conn.mcpEnabled ? 'ok' : 'default'"
              :data-testid="`db-mcp-connection-${conn.id}`"
              >{{
                conn.mcpEnabled ? t('studio.settings.dbMcp.exposed.status') : t('common.off.status')
              }}</Badge
            >
            <Button
              variant="dialog"
              size="kira-lg"
              :disabled="!conn.file"
              :data-testid="`db-mcp-connection-edit-${conn.id}`"
              @click="connectionDialogStore.openFile(conn.file)"
            >
              {{ t('common.action.edit.label') }}
            </Button>
          </div>
        </Card>
      </li>
    </ul>
    <FieldDescription v-else data-testid="db-mcp-connections-empty">
      {{ t('studio.settings.dbMcp.empty.empty') }}
    </FieldDescription>
  </div>
</template>
