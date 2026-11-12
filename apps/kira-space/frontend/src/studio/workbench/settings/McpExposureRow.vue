<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import { Card } from '@theme/components/ui/card';
import { FieldDescription } from '@theme/components/ui/field';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';

// One Studio MCP settings row: what an agent may see (name, status line, Exposed/Off badge) and the
// switch that flips it. Testids derive from `testid`: `-row-`, `-` and `-toggle-` plus the id.
defineProps<{
  id: string;
  name: string;
  enabled: boolean;
  status: string;
  pending: boolean;
  toggleAria: string;
  testid: string;
}>();
defineEmits<{ toggle: [enabled: boolean] }>();
</script>

<template>
  <Card
    variant="list"
    class="flex-row items-center justify-between gap-1.5"
    :data-testid="`${testid}-row-${id}`"
  >
    <div class="flex flex-col min-w-0 gap-0.5">
      <span class="break-words text-fg text-kira-md">{{ name }}</span>
      <FieldDescription v-if="enabled">{{ status }}</FieldDescription>
      <slot name="detail" />
    </div>
    <div class="flex shrink-0 items-center gap-1.5">
      <slot />
      <Badge :variant="enabled ? 'ok' : 'default'" :data-testid="`${testid}-${id}`">{{
        enabled ? t('studio.settings.dbMcp.exposed.status') : t('common.off.status')
      }}</Badge>
      <Switch
        :model-value="enabled"
        :disabled="pending"
        :aria-label="toggleAria"
        :data-testid="`${testid}-toggle-${id}`"
        @update:model-value="(v) => $emit('toggle', v === true)"
      />
      <slot name="actions" />
    </div>
  </Card>
</template>
