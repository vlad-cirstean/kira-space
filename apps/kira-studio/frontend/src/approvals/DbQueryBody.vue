<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbQueryPayloadSchema } from '@shared/domain/dbmcp';
import { FieldDescription } from '@theme/components/ui/field';
import { type MessageKey, t } from '@theme/i18n';
import { computed } from 'vue';
import ApprovalPlanEvidence from './ApprovalPlanEvidence.vue';
import ApprovalStatement from './ApprovalStatement.vue';
import ReviewResultOption from './ReviewResultOption.vue';

const props = defineProps<{ item: ApprovalItem }>();
defineEmits<{ options: [value: { reviewResult: boolean }] }>();

const payload = computed(() => dbQueryPayloadSchema.parse(props.item.payload));

const CLASS_BODY_KEYS: Record<string, MessageKey> = {
  read: 'studio.dbMcp.approval.read.body',
  write: 'studio.dbMcp.approval.write.body',
  ddl: 'studio.dbMcp.approval.ddl.body',
  unknown: 'studio.dbMcp.approval.unknown.body',
};

const RANK: Record<string, number> = { read: 0, write: 1, ddl: 2 };
const ENFORCEMENT_KEYS: Record<string, MessageKey> = {
  session: 'studio.dbMcp.approval.enforcement.session.body',
  app: 'studio.dbMcp.approval.enforcement.app.body',
  none: 'studio.dbMcp.approval.enforcement.none.body',
};

// The statement looks less powerful than the mode the agent declared.
const declaredHigher = computed(
  () => (RANK[payload.value.detected] ?? Infinity) < RANK[payload.value.mode],
);
const masked = computed(() => payload.value.mode === 'masked');
const enforcement = computed(() =>
  payload.value.mode === 'read' || masked.value
    ? ENFORCEMENT_KEYS[payload.value.enforcement]
    : undefined,
);

const lede = computed(() => {
  const params = { connection: props.item.target.name, kind: props.item.target.kind };
  return t(
    CLASS_BODY_KEYS[props.item.operation.class] ?? 'studio.dbMcp.approval.other.body',
    params,
  );
});
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="db-mcp-approval-body">
    <p class="m-0 whitespace-pre-wrap">{{ lede }}</p>
    <FieldDescription v-if="masked" data-testid="db-mcp-approval-mode-masked">
      {{ t('studio.project.connectionDialog.mcp.masked.label') }}
    </FieldDescription>
    <FieldDescription v-if="declaredHigher" data-testid="db-mcp-approval-declared-higher">
      {{
        t('studio.dbMcp.approval.declaredHigher.body', {
          declared: payload.mode,
          detected: payload.detected,
        })
      }}
    </FieldDescription>
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
