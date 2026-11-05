<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useApiAgentAccess } from './state/apiConfig';
import { useApiVariablesDialogStore } from './state/apiVariablesDialog';
import { useVariablesStore } from './state/variables';

// What agents may use through the API request tools: per environment, the variable names and where
// each secret may go. Read-only; Edit opens the environment's dialog on its Agent access tab.
const access = useApiAgentAccess();
const dialogStore = useApiVariablesDialogStore();
const variablesStore = useVariablesStore();

const rows = computed(() =>
  (access.data.value ?? []).map((env) => ({
    ...env,
    file: variablesStore.environments.find((e) => e.id === env.id)?.file ?? null,
  })),
);
</script>

<template>
  <section class="flex min-w-0 flex-col gap-2 p-2" data-testid="api-agent-access">
    <h3 class="m-0 text-kira-lg font-medium">{{
      t('studio.project.connectionDialog.mcp.access.label')
    }}</h3>
    <FieldDescription>{{ t('studio.api.agentAccess.overview.body') }}</FieldDescription>
    <FieldDescription v-if="rows.length === 0" data-testid="api-agent-access-none">
      {{ t('studio.api.agentAccess.none.hint') }}
    </FieldDescription>
    <ul v-else class="m-0 flex list-none flex-col gap-2 p-0">
      <li
        v-for="env in rows"
        :key="env.id"
        class="flex flex-col gap-1"
        :data-testid="`api-agent-access-env-${env.id}`"
      >
        <div class="flex items-center gap-2">
          <span :class="colorMarkClass('dot', env.color)" />
          <span class="min-w-0 flex-1 truncate text-kira-md font-medium">{{ env.name }}</span>
          <Badge v-if="env.autoApproveReads" variant="warn">{{
            t('studio.api.agentAccess.autoReads.status')
          }}</Badge>
          <Button
            v-if="env.file"
            variant="dialog"
            size="kira-lg"
            data-testid="api-agent-access-edit"
            @click="dialogStore.open(env.file, 'agent')"
          >
            {{ t('common.action.edit.label') }}
          </Button>
        </div>
        <ul class="m-0 flex list-none flex-col gap-0.5 p-0 pl-4">
          <li
            v-for="v in env.variables"
            :key="v.name"
            class="flex flex-wrap items-center gap-2 font-data text-kira-sm"
            :data-testid="`api-agent-access-var-${v.name}`"
          >
            <span>{{ v.name }}</span>
            <Badge v-if="v.secret">{{ t('common.secret.label') }}</Badge>
            <span v-if="v.hosts?.length" class="text-muted-foreground">{{
              v.hosts.join(', ')
            }}</span>
          </li>
        </ul>
      </li>
    </ul>
  </section>
</template>
