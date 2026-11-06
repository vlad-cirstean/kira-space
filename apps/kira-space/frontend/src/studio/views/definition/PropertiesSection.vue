<script setup lang="ts">
import type { DefinitionSection } from '@shared/domain/definition';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Table, TableBody, TableCell, TableRow } from '@theme/components/ui/table';
import { t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';

// One `definition.sections[]` entry — a Kafka topic's Partitions/Configuration, a
// consumer group's Group/Members/Committed offsets, an SQS queue's Attributes. Same section chrome
// and count badge IndexesSection.vue already uses, over a generic name/value/muted-detail row list
// rather than a per-kind table, since the adapter (not this component) decides what belongs here.
defineProps<{
  section: DefinitionSection;
}>();
</script>

<template>
  <section
    class="flex flex-col gap-1.5"
    data-testid="definition-properties"
    :data-title="section.title"
  >
    <SectionHeading :label="section.title" :count="section.rows.length" />

    <InlineState v-if="section.rows.length === 0" kind="empty">{{
      t('studio.definition.properties.empty')
    }}</InlineState>

    <Table v-else>
      <TableBody>
        <TableRow v-for="row in section.rows" :key="row.name" data-testid="definition-row">
          <TableCell class="whitespace-nowrap font-data text-muted-foreground">{{
            row.name
          }}</TableCell>
          <TableCell class="def-prop-value font-data">{{ row.value }}</TableCell>
          <TableCell :class="rowMeta">{{ row.detail ?? '' }}</TableCell>
        </TableRow>
      </TableBody>
    </Table>
  </section>
</template>
