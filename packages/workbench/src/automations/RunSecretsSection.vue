<script setup lang="ts">
import type { CustomScript } from '@shared/domain/scripts';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';
import { useAsyncAction } from '../behaviour/asyncAction';
import SecretInput from '../components/SecretInput.vue';
import { useAutomationsModule } from './module';

// The saved values of a run's secret params. A value goes in once and is never shown; the file
// holds only the param's name. Save and Clear act at once, apart from the file text.
const props = defineProps<{ script: CustomScript }>();
const { scripts } = useAutomationsModule();

const secretParams = computed(() => props.script.params.filter((p) => p.secret));
const drafts = ref<Record<string, string>>({});
const target = ref<string | null>(null);
const storageOff = computed(() => !scripts.secretStorage().available);

function isSaved(name: string): boolean {
  return props.script.savedSecrets.includes(name);
}

const action = useAsyncAction(async (name: string, value: string | null) => {
  target.value = name;
  await scripts.setSecrets(props.script.id, { [name]: value });
  drafts.value = { ...drafts.value, [name]: '' };
});
const pending = action.pending;
const error = action.error;
const apply = action.run;
</script>

<template>
  <section v-if="secretParams.length > 0" class="flex flex-col gap-2" data-testid="run-secrets">
    <h3 class="m-0 text-kira-lg font-medium">{{ t('automations.file.secrets.title') }}</h3>
    <FieldDescription>{{ t('automations.file.secrets.hint') }}</FieldDescription>
    <FieldDescription v-if="storageOff" data-testid="run-secrets-off">{{
      t('automations.params.secretValue.storageOff.description')
    }}</FieldDescription>
    <ul class="m-0 flex list-none flex-col gap-2 p-0">
      <li
        v-for="p in secretParams"
        :key="p.name"
        class="flex items-center gap-2"
        :data-testid="`run-secret-${p.name}`"
      >
        <span class="w-32 shrink-0 truncate font-data text-kira-sm">{{ p.name }}</span>
        <SecretInput
          :model-value="drafts[p.name] ?? ''"
          :set="isSaved(p.name)"
          class="min-w-0 flex-1"
          :disabled="storageOff"
          :aria-label="t('automations.script.paramNamed.label', { name: p.name })"
          :data-testid="`run-secret-value-${p.name}`"
          @update:model-value="(v) => (drafts = { ...drafts, [p.name]: String(v) })"
        />
        <Badge v-if="isSaved(p.name)" :data-testid="`run-secret-saved-${p.name}`">{{
          t('automations.params.secretValue.saved.status')
        }}</Badge>
        <Button
          variant="dialog"
          size="kira-lg"
          :disabled="storageOff || (drafts[p.name] ?? '') === ''"
          :pending="pending && target === p.name"
          :data-testid="`run-secret-save-${p.name}`"
          @click="apply(p.name, drafts[p.name] ?? '')"
        >
          {{ t('common.action.save.label') }}
        </Button>
        <Button
          v-if="isSaved(p.name)"
          variant="dialog"
          size="kira-lg"
          :data-testid="`run-secret-clear-${p.name}`"
          @click="apply(p.name, null)"
        >
          {{ t('common.action.clear.label') }}
        </Button>
      </li>
    </ul>
    <Alert v-if="error" variant="error" data-testid="run-secrets-error">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>
  </section>
</template>
