<script setup lang="ts">
import type { DocumentSchemaMeta } from '@shared/domain/definition';
import CodeBlock from '@theme/components/CodeBlock.vue';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Badge } from '@theme/components/ui/badge';
import { Table, TableBody, TableCell, TableRow } from '@theme/components/ui/table';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { jsonSchemaFields } from './structure';

const props = defineProps<{
  documentSchema: DocumentSchemaMeta | null;
}>();

const fields = computed(() =>
  props.documentSchema?.isJsonSchema && props.documentSchema.validator
    ? jsonSchemaFields(props.documentSchema.validator)
    : null,
);

// Raw JSON fallback: no validator, a validator that isn't a $jsonSchema, or a $jsonSchema this
// parser could not turn into field rows (P19 D12) — never an empty table pretending to be one.
const showRaw = computed(() => props.documentSchema?.validator != null && fields.value === null);
</script>

<template>
  <section class="flex flex-col gap-1.5" data-testid="definition-validation">
    <SectionHeading :label="t('studio.definition.validation.heading.label')">
      <span class="flex items-center gap-1.5">
        <Badge v-if="documentSchema?.validationLevel" variant="chip" data>
          {{ documentSchema.validationLevel }}
        </Badge>
        <Badge v-if="documentSchema?.validationAction" variant="chip" data>
          {{ documentSchema.validationAction }}
        </Badge>
      </span>
    </SectionHeading>

    <InlineState v-if="!documentSchema?.validator" kind="empty">
      {{ t('studio.definition.validation.noValidator.empty') }}
    </InlineState>

    <Table v-else-if="fields">
      <TableBody>
        <TableRow v-for="f in fields" :key="f.name" data-testid="definition-row">
          <TableCell class="def-val-name font-data">{{ f.name }}</TableCell>
          <TableCell class="def-val-type font-data">{{ f.bsonType ?? '' }}</TableCell>
          <TableCell class="def-val-required">
            <Badge v-if="f.required">{{ t('common.required.label') }}</Badge>
          </TableCell>
          <TableCell class="text-muted-foreground">{{ f.description ?? '' }}</TableCell>
        </TableRow>
      </TableBody>
    </Table>

    <CodeBlock v-else-if="showRaw">{{ documentSchema?.validator }}</CodeBlock>
  </section>
</template>
