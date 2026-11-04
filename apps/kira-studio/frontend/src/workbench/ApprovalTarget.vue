<script setup lang="ts">
import { type ConnectionKind, connectionKindSchema } from '@shared/domain/connection';
import type { DbMcpApprovalTarget } from '@shared/domain/dbmcp';
import { Badge } from '@theme/components/ui/badge';
import { Card } from '@theme/components/ui/card';
import { connColorVar } from '@theme/connColor';
import { type MessageKey, t } from '@theme/i18n';
import { computed } from 'vue';
import { KIND_ACCENT_CLASS, KIND_LABEL } from '../project/connection/kinds';

const props = defineProps<{
  connectionName: string;
  kind: string;
  target: DbMcpApprovalTarget;
  statementClass: string;
}>();

const CLASS_LABEL_KEYS: Record<string, MessageKey> = {
  read: 'studio.project.connectionDialog.mcp.read.label',
  write: 'studio.project.connectionDialog.mcp.write.label',
  ddl: 'studio.project.connectionDialog.mcp.ddl.label',
  unknown: 'studio.dbMcp.approval.target.unclassified.label',
};

const kindParsed = computed<ConnectionKind | undefined>(
  () => connectionKindSchema.safeParse(props.kind).data,
);
const kindLabel = computed(() => (kindParsed.value ? KIND_LABEL[kindParsed.value] : props.kind));
const kindClass = computed(() => (kindParsed.value ? KIND_ACCENT_CLASS[kindParsed.value] : ''));
const accent = computed(() => connColorVar(props.target.color));
const bannerStyle = computed(() =>
  accent.value
    ? {
        borderLeftColor: accent.value,
        backgroundColor: `color-mix(in srgb, ${accent.value} 15%, transparent)`,
      }
    : undefined,
);

const effective = computed(() => {
  const { path, database } = props.target;
  if (path.length > 0) return path.map((s) => `${s.kind}: ${s.name}`).join(' / ');
  return database || t('studio.dbMcp.approval.target.defaultDatabase.label');
});
const classLabel = computed(() => {
  const key = CLASS_LABEL_KEYS[props.statementClass];
  return key ? t(key) : props.statementClass;
});
const classVariant = computed(() => (props.statementClass === 'read' ? 'default' : 'error'));
</script>

<template>
  <Card
    class="shrink-0 gap-1 border-l-4"
    :style="bannerStyle"
    :aria-label="t('studio.dbMcp.approval.target.aria')"
    data-testid="db-mcp-approval-target"
  >
    <div class="flex items-start justify-between gap-2">
      <span
        class="min-w-0 break-words text-kira-xl font-medium"
        data-testid="db-mcp-approval-target-name"
      >
        {{ connectionName }}
      </span>
      <span class="flex shrink-0 items-center gap-1">
        <Badge v-if="target.readOnly">{{ t('common.state.readOnly.status') }}</Badge>
        <Badge :variant="classVariant" data-testid="db-mcp-approval-target-class">
          {{ classLabel }}
        </Badge>
      </span>
    </div>
    <div class="flex flex-wrap items-baseline gap-x-2 text-kira-sm">
      <span :class="kindClass" data-testid="db-mcp-approval-target-kind">{{ kindLabel }}</span>
      <span v-if="target.host" class="font-data text-muted-foreground">{{ target.host }}</span>
    </div>
    <span
      class="min-w-0 break-all font-data text-kira-lg"
      data-testid="db-mcp-approval-target-database"
    >
      {{ effective }}
    </span>
  </Card>
</template>
