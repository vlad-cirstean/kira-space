<script setup lang="ts">
import { pathTail } from '@shared/domain/tree';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@theme/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { type MessageKey, t } from '@theme/i18n';
import { useTabsStore } from '../../state/tabs';
import type { ConstraintRow } from './structure';

const props = defineProps<{
  connectionId: string;
  constraints: ConstraintRow[];
}>();

// PK/FK get the same yellow/blue letter badge the Columns section and the SQL grid header use
// ("keys are labelled PK/FK, never inferred from colour alone" — SlickGridHost.vue's own
// header carries the same law now) — check/unique/exclusion have no such single-letter convention,
// so they stay a text label.
const KEY_LABEL: Partial<Record<ConstraintRow['type'], 'PK' | 'FK'>> = {
  primaryKey: 'PK',
  foreignKey: 'FK',
  referencedBy: 'FK',
};
const TYPE_LABEL_KEY: Record<ConstraintRow['type'], MessageKey> = {
  primaryKey: 'studio.definition.constraint.type.primaryKey.label',
  unique: 'studio.definition.unique.label',
  foreignKey: 'studio.definition.constraint.type.foreignKey.label',
  check: 'studio.definition.constraint.type.check.label',
  exclusion: 'studio.definition.constraint.type.exclusion.label',
  referencedBy: 'studio.definition.constraint.type.referencedBy.label',
};

function referencedTableName(c: ConstraintRow): string | null {
  if (!c.referencedPath) return null;
  return pathTail(c.referencedPath)?.name ?? c.referencedPath;
}

// Every foreignKey/referencedBy row carries the other table's own path (P7's field, carried
// through by structure.ts's merge) — opening its definition tab is the same "browse structure by
// following the schema" affordance P7 already gives cell-level FK navigation in the grid.
function onNavigate(c: ConstraintRow): void {
  if (c.referencedPath) useTabsStore().openDefinitionTab(props.connectionId, c.referencedPath);
}
</script>

<template>
  <section class="flex flex-col gap-1.5" data-testid="definition-constraints">
    <SectionHeading
      :label="t('studio.definition.constraints.heading.label')"
      :count="constraints.length"
    />
    <Table columns>
      <TableHeader>
        <TableRow class="hover:bg-transparent">
          <TableHead>{{ t('common.field.name.label') }}</TableHead>
          <TableHead>{{ t('common.field.type.label') }}</TableHead>
          <TableHead>{{ t('studio.definition.definition.label') }}</TableHead>
          <TableHead>{{ t('studio.definition.table.label') }}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow
          v-for="c in constraints"
          :key="`${c.type}:${c.name}`"
          data-testid="definition-row"
        >
          <TableCell class="def-con-name">{{ c.name }}</TableCell>
          <TableCell class="def-con-type">
            <span v-if="KEY_LABEL[c.type] === 'PK'" class="text-warn text-kira-sm">{{
              t('studio.definition.primaryKey.label')
            }}</span>
            <span v-else-if="KEY_LABEL[c.type] === 'FK'" class="text-info text-kira-sm">{{
              t('studio.definition.foreignKey.label')
            }}</span>
            <Badge v-else>{{ t(TYPE_LABEL_KEY[c.type]) }}</Badge>
          </TableCell>
          <TableCell class="max-w-80 truncate font-data text-muted-foreground">{{
            c.detail
          }}</TableCell>
          <TableCell class="def-con-table">
            <Tooltip v-if="c.referencedPath">
              <TooltipTrigger as-child>
                <Button variant="link" size="inline" @click="onNavigate(c)">
                  {{ referencedTableName(c) }}
                </Button>
              </TooltipTrigger>
              <TooltipContent>{{
                t('studio.definition.constraint.openReferenced.tooltip', {
                  table: referencedTableName(c) ?? '',
                })
              }}</TooltipContent>
            </Tooltip>
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  </section>
</template>
