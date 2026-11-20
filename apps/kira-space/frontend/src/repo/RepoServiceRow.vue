<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import RunScriptRefPicker from './RunScriptRefPicker.vue';
import type { RepoScriptRefValue } from './state/repoFormTypes';

// One required service of a repo: the Run script that must be running before work starts on a task.
const props = defineProps<{ index: number }>();
const service = defineModel<RepoScriptRefValue | undefined>({ required: true });
const emit = defineEmits<{ remove: [] }>();
</script>

<template>
  <div class="flex items-start gap-1.5" data-testid="repo-service">
    <div class="min-w-0 flex-1">
      <RunScriptRefPicker
        :id="`repo-service-script-${props.index}`"
        v-model="service"
        testid="repo-service"
        place="service"
      />
    </div>
    <TooltipIconButton
      icon="close"
      :label="t('space.repo.config.services.remove.tooltip')"
      data-testid="repo-service-remove"
      @click="emit('remove')"
    />
  </div>
</template>
