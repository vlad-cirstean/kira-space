<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { useId } from 'vue';

// One enabled MCP server's grant: every tool, or the tools typed by name. Kira cannot list a user
// server's tools, so there is no per-tool query.
defineProps<{ allTools: boolean; chosen: readonly string[]; typed: string }>();
const emit = defineEmits<{
  allTools: [on: boolean];
  tool: [name: string, on: boolean];
  typed: [value: string];
  add: [];
}>();
const idBase = useId();
</script>

<template>
  <div class="ml-9 flex flex-col gap-1" data-testid="smart-mcp-tools">
    <div class="flex items-center gap-2">
      <Checkbox
        :id="`${idBase}-all`"
        :model-value="allTools"
        data-testid="smart-mcp-all"
        @update:model-value="(v) => emit('allTools', v === true)"
      />
      <Label :for="`${idBase}-all`">{{ t('automations.smart.mcpAllTools.label') }}</Label>
    </div>
    <template v-if="!allTools">
      <div v-for="name in chosen" :key="name" class="flex items-center gap-1.5">
        <span class="font-data">{{ name }}</span>
        <TooltipIconButton
          icon="close"
          :label="t('automations.smart.remove.tooltip', { name })"
          @click="emit('tool', name, false)"
        />
      </div>
      <div class="flex items-center gap-1">
        <Input
          :model-value="typed"
          :placeholder="t('automations.smart.addTool.placeholder')"
          class="font-data"
          data-testid="smart-mcp-add-input"
          @update:model-value="(v) => emit('typed', String(v))"
          @keydown.enter.prevent="emit('add')"
        />
        <Button variant="dialog" size="kira-lg" data-testid="smart-mcp-add" @click="emit('add')">{{
          t('common.action.add.label')
        }}</Button>
      </div>
    </template>
  </div>
</template>
