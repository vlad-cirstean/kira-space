<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import RepoScriptField from './RepoScriptField.vue';
import type { RepoFileEnvironment } from './state/repoFormTypes';

// One environment of a repo: its name and the script that prints the deployed SHA. Typing a script
// drops the row's run reference; other keys of the row stay.
const props = defineProps<{ index: number }>();
const env = defineModel<RepoFileEnvironment>({ required: true });
const emit = defineEmits<{ remove: [] }>();

function setScript(v: string): void {
  const { deployed_sha_script: _, ...rest } = env.value;
  if (!v) {
    env.value = rest;
    return;
  }
  const { deployed_sha_run: _run, ...keep } = rest;
  env.value = { ...keep, deployed_sha_script: v };
}
</script>

<template>
  <div class="flex flex-col gap-1" data-testid="repo-env">
    <div class="flex items-center gap-1.5">
      <Label :for="`repo-env-name-${props.index}`" class="sr-only">{{
        t('common.environmentName.aria')
      }}</Label>
      <Input
        :id="`repo-env-name-${props.index}`"
        :placeholder="t('space.repo.env.name.example')"
        :model-value="env.name"
        class="w-32 shrink-0 font-data font-medium"
        data-testid="repo-env-name"
        @update:model-value="(v) => (env = { ...env, name: String(v) })"
      />
      <RepoScriptField
        :id="`repo-env-script-${props.index}`"
        :model-value="env.deployed_sha_script ?? ''"
        :run-ref="env.deployed_sha_run"
        :placeholder="t('space.repo.env.script.example')"
        :aria-label="t('space.repo.env.script.aria')"
        testid="repo-env-script"
        @update:model-value="setScript"
      />
      <TooltipIconButton
        icon="close"
        :label="t('space.repo.env.remove.tooltip')"
        data-testid="repo-env-remove"
        @click="emit('remove')"
      />
    </div>
  </div>
</template>
