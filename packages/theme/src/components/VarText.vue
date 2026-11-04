<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import BranchChip from '@theme/components/BranchChip.vue';
import PathChip from '@theme/components/PathChip.vue';
import { t } from '@theme/i18n';
import type { TextPart } from '@theme/varText';

defineProps<{ parts: readonly TextPart[] }>();
</script>

<template>
  <span class="whitespace-pre-line"
    ><template v-for="(p, i) in parts" :key="i">
      <template v-if="typeof p === 'string'">{{ p }}</template>
      <BranchChip
        v-else-if="p.kind === 'branch'"
        :name="p.value"
        :variant="p.variant"
        class="h-auto py-px align-middle"
        :data-var="p.name"
        :aria-label="t('theme.varText.aria', { name: p.name, value: p.value })"
      />
      <PathChip
        v-else-if="p.kind === 'path'"
        :path="p.value"
        :variant="p.variant"
        class="h-auto py-px align-middle"
        :data-var="p.name"
        :aria-label="t('theme.varText.aria', { name: p.name, value: p.value })"
      />
      <span
        v-else
        class="inline-flex items-center gap-0.5 rounded-kira-xs border border-info bg-field px-1 font-data text-info"
        data-testid="var-chip"
        :data-var="p.name"
        :aria-label="t('theme.varText.aria', { name: p.name, value: p.value })"
      >
        <CodiconIcon name="symbol-variable" :size="12" /><span>{{ p.value }}</span>
      </span>
    </template></span
  >
</template>
