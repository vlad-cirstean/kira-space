<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import { t } from '@theme/i18n';
import ConfigSecretsSection, {
  type ConfigSecretRow,
} from '@workbench/config/ConfigSecretsSection.vue';
import { computed } from 'vue';
import { useApiConfig, useDeleteApiSecret, useSetApiSecret } from './state/apiConfig';

// The credentials the files reference; the shared section owns the list and dialogs.
const config = useApiConfig();
const setSecret = useSetApiSecret();
const deleteSecret = useDeleteApiSecret();

const secrets = computed<ConfigSecretRow[]>(() => config.data.value?.secrets ?? []);
const storage = computed(() => config.data.value?.storage ?? null);
</script>

<template>
  <ConfigSecretsSection
    :secrets="secrets"
    :storage="storage"
    :set-secret="
      (name, value) =>
        setSecret
          .mutateAsync({ name, value })
          .then(() => undefined)
          .finally(setSecret.reset)
    "
    :delete-secret="(name) => deleteSecret.mutateAsync(name).then(() => undefined)"
    testid-prefix="api-secret"
  >
    <template #title
      ><SectionHeading flush :label="t('studio.api.config.secrets.title')" /></template
    >
  </ConfigSecretsSection>
</template>
