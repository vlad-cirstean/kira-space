<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { type ConnectionKind, connectionKindSchema } from '@shared/domain/connection';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { KIND_ACCENT_CLASS, KIND_LABEL } from '../project/connection/kinds';

const props = defineProps<{ item: ApprovalItem }>();

const kind = computed<ConnectionKind | undefined>(
  () => connectionKindSchema.safeParse(props.item.target.kind).data,
);
const kindLabel = computed(() => {
  if (kind.value) return KIND_LABEL[kind.value];
  return props.item.target.kind === 'kubernetes'
    ? t('common.product.kubernetes.label')
    : props.item.target.kind;
});
</script>

<template>
  <div class="flex min-w-0 flex-col gap-0.5">
    <span class="min-w-0 break-words text-kira-lg font-medium" data-testid="approval-target-name">
      {{ item.target.name }}
    </span>
    <span class="flex flex-wrap items-baseline gap-x-2 text-kira-sm">
      <span :class="kind ? KIND_ACCENT_CLASS[kind] : ''" data-testid="approval-target-kind">
        {{ kindLabel }}
      </span>
      <span v-if="item.target.location" class="font-data text-muted-foreground">
        {{ item.target.location }}
      </span>
    </span>
  </div>
</template>
