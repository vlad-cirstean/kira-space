<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { type ScriptValue, scriptRunPayloadSchema } from '@shared/domain/dbmcp';
import CodeBlock from '@theme/components/CodeBlock.vue';
import { Badge } from '@theme/components/ui/badge';
import { FieldDescription, FieldLegend } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';

// What the run would execute: the command, its folder, and every value it would see. A secret's
// value never reaches this view; a saved one is named, not shown.
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => scriptRunPayloadSchema.parse(props.item.payload));

function valueText(v: ScriptValue): string {
  if (!v.secret) return v.value;
  return v.saved
    ? t('studio.dbMcp.approval.script.secretSaved.status')
    : t('studio.dbMcp.approval.script.secretUnset.status');
}
</script>

<template>
  <div class="flex flex-col gap-1" data-testid="db-mcp-approval-script">
    <p
      v-if="payload.description"
      class="m-0 whitespace-pre-wrap break-words text-kira-sm"
      data-testid="db-mcp-approval-script-description"
    >
      {{ payload.description }}
    </p>
    <p class="m-0 whitespace-pre-wrap">{{ t('studio.dbMcp.approval.script.lede.body') }}</p>
    <FieldLegend>{{ t('common.field.command.label') }}</FieldLegend>
    <CodeBlock data-testid="db-mcp-approval-script-command">{{ payload.command }}</CodeBlock>
    <FieldDescription data-testid="db-mcp-approval-script-cwd">{{
      t('studio.dbMcp.approval.script.folder.label', { folder: payload.cwd })
    }}</FieldDescription>

    <FieldLegend>{{ t('automations.params.title.label') }}</FieldLegend>
    <ul
      v-if="payload.params.length"
      class="m-0 flex list-none flex-col gap-0.5 p-0"
      data-testid="db-mcp-approval-script-params"
    >
      <li
        v-for="p in payload.params"
        :key="p.name"
        class="flex items-center gap-2 font-data text-kira-sm"
        :data-testid="`db-mcp-approval-script-param-${p.name}`"
      >
        <span>{{ p.name }}</span>
        <span class="min-w-0 break-all text-muted-foreground">{{ valueText(p) }}</span>
        <Badge v-if="p.secret">{{ t('common.secret.label') }}</Badge>
      </li>
    </ul>
    <FieldDescription v-else>{{ t('common.state.none.option') }}</FieldDescription>

    <FieldLegend>{{ t('docker.field.env.label') }}</FieldLegend>
    <ul
      v-if="payload.env.length"
      class="m-0 flex list-none flex-col gap-0.5 p-0"
      data-testid="db-mcp-approval-script-env"
    >
      <li
        v-for="e in payload.env"
        :key="e.name"
        class="flex items-center gap-2 font-data text-kira-sm"
        :data-testid="`db-mcp-approval-script-env-${e.name}`"
      >
        <span>{{ e.name }}</span>
        <span class="min-w-0 break-all text-muted-foreground">{{ valueText(e) }}</span>
        <Badge v-if="e.secret">{{ t('common.secret.label') }}</Badge>
      </li>
    </ul>
    <FieldDescription v-else>{{ t('common.state.none.option') }}</FieldDescription>
  </div>
</template>
