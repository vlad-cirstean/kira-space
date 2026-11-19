<script setup lang="ts">
import type { ConnectionFileMcp } from '@shared/domain/connectionConfig';
import { BROWSE_KINDS } from '@shared/domain/dbmcp';
import { FieldDescription, FieldLegend, FieldSet } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { useSettingsStore } from '../../../state/settings';
import ReviewChoiceField from '../../components/ReviewChoiceField.vue';

// Result review per mode: which run_query results Kira holds for the user before the agent gets them.
const props = defineProps<{
  mcp: ConnectionFileMcp;
  kind: string;
  enabled: boolean;
  writeConsole: boolean;
}>();
const emit = defineEmits<{ change: [change: Partial<ConnectionFileMcp>] }>();
const settingsStore = useSettingsStore();

const MODES = [
  { cls: 'read', label: 'studio.project.connectionDialog.mcp.read.label' },
  { cls: 'write', label: 'studio.project.connectionDialog.mcp.write.label' },
  { cls: 'ddl', label: 'studio.project.connectionDialog.mcp.ddl.label' },
  { cls: 'masked', label: 'studio.project.connectionDialog.mcp.masked.label' },
] as const;
type Mode = (typeof MODES)[number]['cls'];

function isDisabled(cls: Mode): boolean {
  return (
    !props.enabled ||
    (cls === 'masked' ? !BROWSE_KINDS.has(props.kind) : cls !== 'read' && !props.writeConsole)
  );
}

// A mode left out of mcp.review is inherit.
function reviewOf(cls: Mode): string {
  return props.mcp.review?.[cls] ?? 'inherit';
}

// Writes the resolved values with inherit left out.
function setValue(cls: Mode, value: string): void {
  const next: Record<string, string> = {};
  for (const m of MODES) {
    const v = m.cls === cls ? value : reviewOf(m.cls);
    if (v !== 'inherit') next[m.cls] = v;
  }
  emit('change', { review: Object.keys(next).length > 0 ? next : undefined });
}
</script>

<template>
  <FieldSet class="flex-1">
    <FieldLegend>{{ t('studio.project.connectionDialog.mcp.review.label') }}</FieldLegend>
    <FieldDescription class="w-full">{{
      t('studio.project.connectionDialog.mcp.reviewNote.label')
    }}</FieldDescription>
    <ReviewChoiceField
      v-for="m in MODES"
      :key="m.cls"
      :label="t(m.label)"
      :app-on="settingsStore.dbMcp.previewResponses"
      :disabled="isDisabled(m.cls)"
      :testid="`connection-mcp-review-${m.cls}`"
      :model-value="reviewOf(m.cls)"
      @update:model-value="(v) => setValue(m.cls, v)"
    />
    <FieldDescription class="w-full">{{
      t('studio.project.connectionDialog.mcp.review.description')
    }}</FieldDescription>
    <FieldDescription v-if="enabled" class="w-full" data-testid="connection-mcp-review-enforce">
      {{ t('studio.project.connectionDialog.mcp.reviewEnforceWho.body') }}
      {{ t('studio.project.connectionDialog.mcp.reviewEnforceDeny.body') }}
      {{ t('studio.project.connectionDialog.mcp.reviewEnforceMemory.body') }}
      {{ t('studio.project.connectionDialog.mcp.reviewEnforceLimits.body') }}
      {{ t('studio.project.connectionDialog.mcp.reviewEnforceScope.body') }}
    </FieldDescription>
  </FieldSet>
</template>
