<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbExemptionPayloadSchema } from '@shared/domain/dbmcp';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import ApprovalStatement from './ApprovalStatement.vue';
import DbParamsList from './DbParamsList.vue';
import DbScriptBlock from './DbScriptBlock.vue';
import KeptVariablesPanel from './KeptVariablesPanel.vue';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => dbExemptionPayloadSchema.parse(props.item.payload));
</script>

<template>
  <div class="flex flex-col gap-1">
    <KeptVariablesPanel
      v-if="payload.kept.length"
      :uses="payload.kept"
      :pair-name="() => ''"
      :title="t('studio.kept.panel.query.label')"
      testid-prefix="db-approval"
    />
    <ApprovalStatement
      :kind="item.target.kind"
      :statement="payload.statement"
      :kept="payload.kept"
      data-testid="db-mcp-exemption-body"
    />
    <DbParamsList
      v-if="payload.params.length"
      :params="payload.params"
      :placeholders="payload.placeholders"
      :kept="payload.kept"
    />
    <DbScriptBlock v-if="payload.script" :script="payload.script" :kept="payload.kept" />
  </div>
</template>
