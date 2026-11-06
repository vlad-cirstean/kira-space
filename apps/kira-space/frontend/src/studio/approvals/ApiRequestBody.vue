<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { apiRequestPayloadSchema } from '@shared/domain/dbmcp';
import CodeBlock from '@theme/components/CodeBlock.vue';
import { FieldDescription, FieldLegend } from '@theme/components/ui/field';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import ApiPairsTable from './ApiPairsTable.vue';

// What the agent would send: the URL, the headers with credentials as {{name}}, and the body. A
// credential's value never reaches this view, and a secret variable shows as "••••".
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => apiRequestPayloadSchema.parse(props.item.payload));
const isRead = computed(() => ['GET', 'HEAD', 'OPTIONS'].includes(payload.value.method));

const body = computed(() => {
  const b = payload.value.body;
  if (!b || b.form || !/json/i.test(b.contentType) || b.truncated) return b;
  try {
    return { ...b, text: JSON.stringify(JSON.parse(b.text), null, 2) };
  } catch {
    return b;
  }
});
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="api-approval">
    <CodeBlock data-testid="api-approval-url">{{ payload.method }} {{ payload.url }}</CodeBlock>
    <FieldDescription class="flex items-center gap-2" data-testid="api-approval-target">
      <template v-if="payload.environment">
        <span :class="colorMarkClass('dot', payload.environment.color)" />
        <span>{{ payload.environment.name }}</span>
      </template>
      <span v-else>{{ t('studio.api.environments.none.option') }}</span>
      <span class="font-data">{{ payload.host }}</span>
    </FieldDescription>

    <p
      v-for="c in payload.credentials"
      :key="c.name"
      class="m-0 text-kira-sm"
      :class="isRead ? 'text-fg' : 'text-warn'"
      :data-testid="`api-approval-credential-${c.name}`"
    >
      {{
        t('studio.dbMcp.approval.api.credential.label', {
          name: c.name,
          hosts: c.hosts.join(', '),
        })
      }}
    </p>

    <ApiPairsTable
      :title="t('studio.console.plan.root.query.label')"
      :rows="payload.query"
      testid="api-approval-query"
    />
    <ApiPairsTable
      :title="t('studio.http.pane.headers.label')"
      :rows="payload.headers"
      testid="api-approval-headers"
    />

    <div v-if="body" class="flex flex-col gap-1" data-testid="api-approval-body">
      <FieldLegend>{{
        t('studio.dbMcp.approval.api.body.label', { type: body.contentType })
      }}</FieldLegend>
      <ApiPairsTable
        v-if="body.form"
        :title="t('studio.dbMcp.approval.api.form.label')"
        :rows="body.form"
        testid="api-approval-form"
      />
      <CodeBlock v-else data-testid="api-approval-body-text">{{ body.text }}</CodeBlock>
      <FieldDescription v-if="body.truncated" data-testid="api-approval-body-truncated">{{
        t('studio.dbMcp.approval.api.truncated.status')
      }}</FieldDescription>
    </div>

    <ApiPairsTable
      :title="t('studio.dbMcp.approval.api.variables.label')"
      :rows="payload.variables"
      testid="api-approval-variables"
    />
    <FieldDescription data-testid="api-approval-options">{{
      t('studio.dbMcp.approval.api.options.label', {
        redirects: payload.followRedirects
          ? t('studio.dbMcp.approval.api.redirectsOn.option')
          : t('studio.dbMcp.approval.api.redirectsOff.option'),
        seconds: payload.timeoutSeconds,
      })
    }}</FieldDescription>
  </div>
</template>
