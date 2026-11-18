<script setup lang="ts" generic="T">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';

defineProps<{ items: T[]; addLabel: string; disabled?: boolean; testid: string }>();
const emit = defineEmits<{ add: []; remove: [index: number] }>();
</script>

<template>
  <div class="flex flex-col gap-1.5" :data-testid="testid">
    <div
      v-for="(item, index) in items"
      :key="index"
      class="flex items-center gap-1.5"
      data-testid="docker-edit-row"
    >
      <div class="flex min-w-0 flex-1 items-center gap-1.5">
        <slot name="row" :item="item" :index="index" />
      </div>
      <TooltipIconButton
        icon="trash"
        :label="t('common.action.remove.label')"
        :disabled="disabled"
        data-testid="docker-edit-row-remove"
        @click="emit('remove', index)"
      />
    </div>
    <Button
      size="kira"
      variant="toolbar"
      class="self-start"
      :disabled="disabled"
      data-testid="docker-edit-row-add"
      @click="emit('add')"
      ><CodiconIcon :name="ICON.add" :size="13" />
      {{ addLabel }}
    </Button>
  </div>
</template>
