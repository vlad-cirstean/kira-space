<script setup lang="ts">
import type { ConnectionFileMcp } from '@shared/domain/connectionConfig';
import { BROWSE_KINDS } from '@shared/domain/dbmcp';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { type MessageKey, t } from '@theme/i18n';
import { computed } from 'vue';

// browse_table's permission. An unset value follows read.
const props = defineProps<{ mcp: ConnectionFileMcp; kind: string; enabled: boolean }>();
const emit = defineEmits<{ change: [change: Partial<ConnectionFileMcp>] }>();

const supported = computed(() => BROWSE_KINDS.has(props.kind));
const MODE_LABELS: Record<string, MessageKey> = {
  deny: 'common.action.deny.label',
  allow: 'studio.project.connectionDialog.mcp.allow.label',
  prompt: 'studio.project.connectionDialog.mcp.prompt.label',
};
const readMode = computed(() => props.mcp.read ?? 'allow');
const current = computed(() => props.mcp.masked ?? 'inherit');

const items = computed(() =>
  [
    {
      value: 'inherit',
      label: t('studio.project.connectionDialog.mcp.maskedInherit.option', {
        mode: t(MODE_LABELS[readMode.value] ?? MODE_LABELS.deny),
      }),
    },
    { value: 'deny', label: t(MODE_LABELS.deny) },
    { value: 'allow', label: t(MODE_LABELS.allow) },
    { value: 'prompt', label: t(MODE_LABELS.prompt) },
  ].map((i) => ({
    ...i,
    testid: `connection-mcp-masked-${i.value}`,
    disabled: !props.enabled || !supported.value,
  })),
);

function setValue(value: string): void {
  emit('change', { masked: value === 'inherit' ? undefined : value });
}
</script>

<template>
  <Field class="flex-1" data-testid="connection-mcp-masked">
    <FieldLabel
      >{{ t('studio.project.connectionDialog.mcp.masked.label') }}
      <span class="text-subtle"
        >— {{ t('studio.project.connectionDialog.mcp.maskedNote.label') }}</span
      ></FieldLabel
    >
    <SecondaryTabs
      variant="segmented"
      size="kira-lg"
      :model-value="current"
      :items="items"
      data-testid="connection-mcp-masked-value"
      @update:model-value="(v) => setValue(String(v))"
    />
    <FieldDescription v-if="!supported" class="w-full" data-testid="connection-mcp-masked-na">
      {{ t('studio.project.connectionDialog.mcp.maskedNa.body') }}
    </FieldDescription>
    <FieldDescription
      v-else-if="enabled"
      class="w-full"
      data-testid="connection-mcp-masked-enforce"
    >
      {{ t('studio.project.connectionDialog.mcp.maskedEnforceWho.body') }}
      {{ t('studio.project.connectionDialog.mcp.maskedEnforceHit.body') }}
      {{ t('studio.project.connectionDialog.mcp.maskedEnforceLimits.body') }}
    </FieldDescription>
  </Field>
</template>
