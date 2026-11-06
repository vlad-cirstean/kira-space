<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbExemptionPayloadSchema } from '@shared/domain/dbmcp';
import { FieldDescription } from '@theme/components/ui/field';
import { type MessageKey, t } from '@theme/i18n';
import { formatCount, formatDuration } from '@workbench/behaviour/format';
import { computed } from 'vue';
import ApprovalPlanEvidence from './ApprovalPlanEvidence.vue';
import ApprovalStatement from './ApprovalStatement.vue';
import ReviewResultOption from './ReviewResultOption.vue';

const props = defineProps<{ item: ApprovalItem }>();
defineEmits<{ options: [value: { reviewResult: boolean }] }>();

const payload = computed(() => dbExemptionPayloadSchema.parse(props.item.payload));

const LIMIT_LABEL: Record<string, MessageKey> = {
  statement_timeout: 'studio.dbMcp.approval.exemption.statementTimeout.label',
  expensive_query: 'studio.dbMcp.approval.exemption.expensiveQuery.label',
};
const ENFORCEMENT_KEYS: Record<string, MessageKey> = {
  session: 'studio.dbMcp.approval.enforcement.session.body',
  app: 'studio.dbMcp.approval.enforcement.app.body',
  none: 'studio.dbMcp.approval.enforcement.none.body',
};

const lede = computed(() =>
  t('studio.dbMcp.approval.exemption.lede.body', {
    connection: props.item.target.name,
    kind: props.item.target.kind,
  }),
);
const enforcement = computed(() =>
  payload.value.mode === 'read' || payload.value.mode === 'masked'
    ? ENFORCEMENT_KEYS[payload.value.enforcement]
    : undefined,
);
const noTimeout = computed(() => payload.value.limits.some((l) => l.limit === 'statement_timeout'));
const limits = computed(() =>
  payload.value.limits.map((l) => {
    const show = l.unit === 'ms' ? formatDuration : formatCount;
    const labelKey = LIMIT_LABEL[l.limit];
    return {
      limit: l.limit,
      text: t('studio.dbMcp.approval.exemption.limit.label', {
        label: labelKey ? t(labelKey) : l.limit,
        measured: show(l.measured),
        max: show(l.max),
      }),
    };
  }),
);
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="db-mcp-exemption-body">
    <p class="m-0 whitespace-pre-wrap" data-testid="db-mcp-exemption-lede">{{ lede }}</p>
    <FieldDescription
      v-for="l in limits"
      :key="l.limit"
      :data-limit="l.limit"
      data-testid="db-mcp-exemption-limit"
    >
      {{ l.text }}
    </FieldDescription>
    <FieldDescription v-if="noTimeout" data-testid="db-mcp-exemption-no-timeout">
      {{ t('studio.dbMcp.approval.exemption.noTimeout.body') }}
    </FieldDescription>
    <div class="flex flex-col gap-1" data-testid="db-mcp-exemption-reason">
      <FieldDescription>{{ t('studio.dbMcp.approval.exemption.reason.label') }}</FieldDescription>
      <p class="m-0 whitespace-pre-wrap border-l-2 pl-2">{{ payload.reason }}</p>
    </div>
    <FieldDescription v-if="enforcement" data-testid="db-mcp-approval-enforcement">
      {{ t(enforcement) }}
    </FieldDescription>
    <ApprovalStatement :kind="item.target.kind" :statement="payload.statement" />
    <FieldDescription data-testid="db-mcp-approval-detected">
      {{ t('studio.dbMcp.approval.detected.label', { class: payload.detected }) }}
    </FieldDescription>
    <ApprovalPlanEvidence v-if="payload.plan" :plan="payload.plan" />
    <ReviewResultOption
      v-if="payload.reviewOffered"
      :mode="payload.mode"
      :reviewed="payload.reviewResult"
      @options="(v) => $emit('options', v)"
    />
  </div>
</template>
