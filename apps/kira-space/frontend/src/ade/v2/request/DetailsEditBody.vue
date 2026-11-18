<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import CodeBlock from '@theme/components/CodeBlock.vue';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Label } from '@theme/components/ui/label';
import { type MessageKey, t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';
import { detailsEditPayloadSchema } from './payload';

// One approvals item for a task's pending file edits. Every file starts approved; unchecking one restores
// its held copy. The agent's reason heads the list.
const props = defineProps<{ item: ApprovalItem }>();
const emit = defineEmits<{ options: [{ approved: string[] }] }>();

const payload = computed(() => detailsEditPayloadSchema.parse(props.item.payload));
const unchecked = ref<string[]>([]);
const approved = computed(() =>
  payload.value.items.map((f) => f.path).filter((p) => !unchecked.value.includes(p)),
);

watch(approved, () => emit('options', { approved: approved.value }), { immediate: true });

function toggle(path: string): void {
  unchecked.value = unchecked.value.includes(path)
    ? unchecked.value.filter((u) => u !== path)
    : [...unchecked.value, path];
}
</script>

<template>
  <div class="flex flex-col gap-3" data-testid="ade-details-edit-body">
    <p v-if="payload.reason" data-testid="ade-details-edit-reason">
      <span class="text-muted-foreground">{{ t('common.field.reason.label') }}</span>
      {{ payload.reason }}
    </p>
    <div
      v-for="file in payload.items"
      :key="file.path"
      class="flex flex-col gap-1"
      data-testid="ade-details-edit-file"
    >
      <div class="flex items-center gap-2">
        <Checkbox
          :id="`details-edit-${file.path}`"
          :model-value="!unchecked.includes(file.path)"
          @update:model-value="toggle(file.path)"
        />
        <Label :for="`details-edit-${file.path}`" class="min-w-0 flex-1 truncate">
          <code>{{ file.path }}</code>
        </Label>
        <span class="text-muted-foreground">{{
          t(`space.ade.detailsEdit.cause.${file.cause}.label` as MessageKey)
        }}</span>
      </div>
      <p v-if="file.collision" class="text-muted-foreground">
        {{ t('space.ade.detailsEdit.collision.body', { by: file.collision }) }}
      </p>
      <CodeBlock v-if="file.text" class="max-h-48 overflow-auto">{{ file.text }}</CodeBlock>
    </div>
  </div>
</template>
