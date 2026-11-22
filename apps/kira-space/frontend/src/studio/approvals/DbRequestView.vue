<script setup lang="ts">
import type { KeptUse } from '@shared/domain/kept';
import ApprovalStatement from './ApprovalStatement.vue';
import DbParamsList from './DbParamsList.vue';

// The statement as the agent sent it, with the kept variables bound to its placeholders: read-only, shared by
// the prompt and the review's Request section.
withDefaults(
  defineProps<{
    kind: string;
    statement: string;
    kept: readonly KeptUse[];
    params: readonly string[];
    placeholders: readonly string[];
    statementTestid?: string;
  }>(),
  { statementTestid: 'db-mcp-approval-body' },
);
</script>

<template>
  <div class="flex flex-col gap-1">
    <ApprovalStatement
      :kind="kind"
      :statement="statement"
      :kept="kept"
      :data-testid="statementTestid"
    />
    <DbParamsList v-if="params.length" :params="params" :placeholders="placeholders" :kept="kept" />
  </div>
</template>
