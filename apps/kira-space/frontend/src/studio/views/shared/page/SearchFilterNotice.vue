<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { usePageSearchFilterStore } from './searchFilter';

defineProps<{ filterKey: string; showAllTestid: string }>();

const pageSearchFilterStore = usePageSearchFilterStore();
</script>

<template>
  <EmptyState :icon="ICON.search" :title="t('studio.common.noMatchingRows.empty')">
    <Button
      variant="toolbar"
      size="kira"
      :data-testid="showAllTestid"
      @click="pageSearchFilterStore.setSearchFiltering(filterKey, false)"
    >
      {{ t('studio.search.showAllRows.label') }}
    </Button>
  </EmptyState>
</template>
