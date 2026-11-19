<script setup lang="ts">
import { t } from '@theme/i18n';
import ConfigSecretsSection from '@workbench/config/ConfigSecretsSection.vue';
import { useDeleteAgentSecret, useSetAgentSecret } from './queries';
import type { AgentSettingsView } from './types';

// The credentials the file references; the shared section owns the list and dialogs.
defineProps<{ view: AgentSettingsView }>();

const setSecret = useSetAgentSecret();
const deleteSecret = useDeleteAgentSecret();
</script>

<template>
  <ConfigSecretsSection
    :secrets="view.secrets"
    :storage="view.storage"
    :set-secret="(name, value) => setSecret.mutateAsync({ name, value }).then(() => undefined)"
    :delete-secret="(name) => deleteSecret.mutateAsync(name).then(() => undefined)"
    testid-prefix="ade-settings-secret"
  >
    <template #description="{ ref }">{{ t('space.ade.settings.secrets.hint', { ref }) }}</template>
  </ConfigSecretsSection>
</template>
