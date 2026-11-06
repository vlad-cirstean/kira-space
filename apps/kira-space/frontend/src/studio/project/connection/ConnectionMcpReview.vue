<script setup lang="ts">
import type { ConnectionFileMcp } from '@shared/domain/connectionConfig';
import { BROWSE_KINDS } from '@shared/domain/dbmcp';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useSettingsStore } from '../../../state/settings';

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

// Inherit's label shows the current app setting.
const items = computed(() => [
  {
    value: 'inherit',
    label: t(
      settingsStore.dbMcp.previewResponses
        ? 'studio.project.connectionDialog.mcp.reviewInheritOn.option'
        : 'studio.project.connectionDialog.mcp.reviewInheritOff.option',
    ),
  },
  { value: 'review', label: t('studio.project.connectionDialog.mcp.reviewAlways.option') },
  { value: 'send', label: t('studio.project.connectionDialog.mcp.reviewNever.option') },
]);

function optionsOf(cls: Mode) {
  return items.value.map((i) => ({
    ...i,
    testid: `connection-mcp-review-${cls}-${i.value}`,
    disabled:
      !props.enabled ||
      (cls === 'masked' ? !BROWSE_KINDS.has(props.kind) : cls !== 'read' && !props.writeConsole),
  }));
}

// A mode left out of mcp.review follows the legacy review_results, else inherit.
function reviewOf(cls: Mode): string {
  return props.mcp.review?.[cls] ?? props.mcp.review_results ?? 'inherit';
}

// Writes the resolved values with inherit left out, and retires the legacy single key.
function setValue(cls: Mode, value: string): void {
  const next: Record<string, string> = {};
  for (const m of MODES) {
    const v = m.cls === cls ? value : reviewOf(m.cls);
    if (v !== 'inherit') next[m.cls] = v;
  }
  emit('change', {
    review: Object.keys(next).length > 0 ? next : undefined,
    review_results: undefined,
  });
}
</script>

<template>
  <Field class="flex-1">
    <FieldLabel
      >{{ t('studio.project.connectionDialog.mcp.review.label') }}
      <span class="text-subtle"
        >— {{ t('studio.project.connectionDialog.mcp.reviewNote.label') }}</span
      ></FieldLabel
    >
    <div v-for="m in MODES" :key="m.cls" class="flex flex-col gap-1">
      <FieldDescription>{{ t(m.label) }}</FieldDescription>
      <SecondaryTabs
        variant="segmented"
        size="kira-lg"
        :model-value="reviewOf(m.cls)"
        :items="optionsOf(m.cls)"
        :data-testid="`connection-mcp-review-${m.cls}`"
        @update:model-value="(v) => setValue(m.cls, String(v))"
      />
    </div>
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
  </Field>
</template>
