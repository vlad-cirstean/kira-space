<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import CodeBlock from '@theme/components/CodeBlock.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { FieldDescription, FieldLegend, FieldSet } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { computed } from 'vue';
import { useCopy } from '../../behaviour/clipboard';
import QueryState from '../../components/QueryState.vue';
import { useInstallMemoryMcp, useMemoryMcpStatus } from '../queries';

// P201: registers the kira-memory stdio server with the agent, or shows the command to run.
const status = useMemoryMcpStatus();
const install = useInstallMemoryMcp();
const command = computed(() => status.data.value?.command ?? '');
const { copy, copied } = useCopy();

const message = computed(() => {
  const r = install.data.value;
  if (!r) return null;
  switch (r.outcome) {
    case 'installed':
      return t('common.agentRegistered.description');
    case 'notFound':
      return t('common.agentCliNotFound.error');
    default:
      return t('common.agentRegistrationRefused.error', { detail: r.detail });
  }
});
</script>

<template>
  <FieldSet data-testid="memory-mcp-section">
    <FieldLegend>{{ t('memory.mcp.connect.title') }}</FieldLegend>
    <FieldDescription>{{ t('memory.mcp.connect.description') }}</FieldDescription>
    <QueryState :query="status" variant="inline" :is-empty="() => false">
      <template #error="{ message: statusMessage }">
        <Alert variant="error">
          <AlertDescription>{{ statusMessage }}</AlertDescription>
        </Alert>
      </template>
      <CodeBlock v-if="command" class="select-all" data-testid="memory-mcp-command">{{
        command
      }}</CodeBlock>
    </QueryState>
    <FieldDescription v-if="command">
      <CatalogText k="memory.mcp.command.hint">
        <template #command><code class="font-data">memory-mcp</code></template>
      </CatalogText>
    </FieldDescription>
    <FieldDescription v-if="message" data-testid="memory-mcp-install-outcome">{{
      message
    }}</FieldDescription>
    <div class="flex items-center gap-1">
      <Button
        variant="dialog"
        size="kira-lg"
        :disabled="!command"
        data-testid="memory-mcp-copy"
        @click="copy(command)"
      >
        {{ copied ? t('common.copy.copied.announce') : t('common.copyCommand.label') }}
      </Button>
      <Button
        variant="dialog-primary"
        size="kira-lg"
        :pending="install.isPending.value"
        :disabled="!status.data.value?.agentAvailable"
        data-testid="memory-mcp-install"
        @click="install.mutate()"
      >
        <CodiconIcon name="plug" :size="13" /> {{ t('common.registerWithAgent.label') }}
      </Button>
    </div>
  </FieldSet>
</template>
