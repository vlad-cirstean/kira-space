<script setup lang="ts">
import type { IndexMeta } from '@shared/domain/tree';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Badge } from '@theme/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@theme/components/ui/table';
import { t } from '@theme/i18n';

defineProps<{
  indexes: IndexMeta[];
}>();
</script>

<template>
  <section class="flex flex-col gap-1.5" data-testid="definition-indexes">
    <SectionHeading :label="t('studio.definition.indexes.heading.label')" :count="indexes.length" />
    <Table columns>
      <TableHeader>
        <TableRow class="hover:bg-transparent">
          <TableHead>{{ t('common.field.name.label') }}</TableHead>
          <TableHead>{{ t('common.field.kind.label') }}</TableHead>
          <TableHead>{{ t('studio.definition.method.label') }}</TableHead>
          <TableHead>{{ t('studio.common.columns.label') }}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow v-for="idx in indexes" :key="idx.name" data-testid="definition-row">
          <TableCell class="def-idx-name">{{ idx.name }}</TableCell>
          <TableCell class="def-idx-badges">
            <Badge v-if="idx.primary">{{ t('studio.definition.index.primary.label') }}</Badge>
            <Badge v-else-if="idx.unique">{{ t('studio.definition.unique.label') }}</Badge>
          </TableCell>
          <TableCell class="def-idx-method font-data">{{ idx.method ?? '' }}</TableCell>
          <TableCell class="font-data text-muted-foreground"
            >({{ idx.columns.join(', ') }})</TableCell
          >
        </TableRow>
      </TableBody>
    </Table>
  </section>
</template>
