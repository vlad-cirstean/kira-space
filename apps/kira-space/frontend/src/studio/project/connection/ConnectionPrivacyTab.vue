<script setup lang="ts">
import type { ConnectionFileModel } from '@shared/domain/connectionConfig';
import type { MaskKind } from '@shared/domain/mask';
import { MASK_REVIEW_KINDS, type MaskReviewColumn } from '@shared/domain/maskReview';
import { Card, CardHeader, CardTitle } from '@theme/components/ui/card';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import ConnectionMaskReview from './ConnectionMaskReview.vue';
import MaskKeySection from './MaskKeySection.vue';
import MaskRuleList from './MaskRuleList.vue';
import { defaultCorrelateFor } from './maskKinds';
import PrivacyLimits from './PrivacyLimits.vue';
import PrivacyOverview from './PrivacyOverview.vue';

// The file's masking rules. They are part of the draft and save with the file; the matching-tag key
// is separate and regenerates at once. A rule masks every column with its name, in every table.
const props = defineProps<{
  /** The connection's row id; empty while the file was never valid. */
  connectionId: string;
  errorFor: (pointer: string) => string | undefined;
}>();
const file = defineModel<ConnectionFileModel>({ required: true });

const rules = computed(() => file.value.masks ?? []);

function hasRule(column: string): boolean {
  const name = column.trim().toLowerCase();
  return rules.value.some((r) => r.column.toLowerCase() === name);
}

function addRule(column: string, kind: MaskKind): void {
  if (hasRule(column)) return;
  file.value.masks = [
    ...rules.value,
    { column, kind, ...(defaultCorrelateFor(kind) ? {} : { correlate: false }) },
  ];
}

function maskFromReview(col: MaskReviewColumn, kind: MaskKind): void {
  addRule(col.column, kind);
}
</script>

<template>
  <div class="flex flex-col gap-3 p-3" data-testid="connection-section-privacy">
    <PrivacyOverview
      :rule-count="rules.length"
      :review-applies="MASK_REVIEW_KINDS.has(file.kind)"
      :review-on="file.mask_review !== undefined"
      :mcp-enabled="file.mcp?.enabled === true"
    />

    <Card variant="section">
      <CardHeader>
        <CardTitle>{{ t('studio.project.connectionDialog.privacy.rules.title') }}</CardTitle>
      </CardHeader>
      <div class="flex flex-col gap-2 px-3 pb-3">
        <FieldDescription class="w-full">{{
          t('studio.project.connectionDialog.privacy.rules.description')
        }}</FieldDescription>
        <MaskRuleList
          :model-value="rules"
          :error-for="props.errorFor"
          :has-rule="hasRule"
          @update:model-value="(v) => (file.masks = v)"
          @add="addRule"
        />
        <MaskKeySection v-if="rules.length > 0" :connection-id="connectionId" :kind="file.kind" />
      </div>
    </Card>

    <ConnectionMaskReview v-model="file" :connection-id="connectionId" @mask="maskFromReview" />

    <PrivacyLimits />
  </div>
</template>
