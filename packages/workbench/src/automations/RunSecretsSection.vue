<script setup lang="ts">
import type { CustomScript } from '@shared/domain/scripts';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import ConfigSecretsSection from '../config/ConfigSecretsSection.vue';
import { useAutomationsModule } from './module';

// The saved values of a run's secret params, as the config secrets list: the file holds only the
// param's name, a value goes in once through the set dialog and is never shown.
const props = defineProps<{ script: CustomScript }>();
const { scripts } = useAutomationsModule();

const secretParams = computed(() => props.script.params.filter((p) => p.secret));
const secrets = computed(() =>
  secretParams.value.map((p) => ({
    name: p.name,
    set: props.script.savedSecrets.includes(p.name),
    referenced: true,
  })),
);
const storage = computed(() => ({
  available: scripts.secretStorage().available,
  reason: t('automations.params.secretValue.storageOff.description'),
}));
</script>

<template>
  <ConfigSecretsSection
    v-if="secretParams.length > 0"
    :secrets="secrets"
    :storage="storage"
    :set-secret="(name, value) => scripts.setSecrets(script.id, { [name]: value })"
    :delete-secret="(name) => scripts.setSecrets(script.id, { [name]: null })"
    testid-prefix="run-secret"
  >
    <template #title
      ><SectionHeading flush :label="t('automations.file.secrets.title')" /></template
    >
    <template #description>{{ t('automations.file.secrets.hint') }}</template>
  </ConfigSecretsSection>
</template>
