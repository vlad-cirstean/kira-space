<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import { FieldLegend } from '@theme/components/ui/field';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  tableHeadRowClass,
} from '@theme/components/ui/table';
import { t } from '@theme/i18n';

// A name/value list of an API request, values exactly as the agent sent them.
defineProps<{
  title: string;
  rows: { name: string; value: string; masked?: boolean; secret?: boolean }[];
  testid: string;
}>();
</script>

<template>
  <div v-if="rows.length" class="flex flex-col gap-1" :data-testid="testid">
    <FieldLegend>{{ title }}</FieldLegend>
    <Table>
      <TableHeader>
        <TableRow :class="tableHeadRowClass">
          <TableHead>{{ t('common.field.name.label') }}</TableHead>
          <TableHead>{{ t('common.field.value.label') }}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow v-for="(row, i) in rows" :key="`${i}-${row.name}`">
          <TableCell class="font-data text-kira-sm">{{ row.name }}</TableCell>
          <TableCell class="break-all font-data text-kira-sm">
            {{ row.value }}
            <Badge v-if="row.masked || row.secret" class="ml-1">{{
              t('common.secret.label')
            }}</Badge>
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  </div>
</template>
