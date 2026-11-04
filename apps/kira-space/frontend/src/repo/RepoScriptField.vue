<script setup lang="ts">
import { FieldDescription } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import type { RepoScriptRefValue } from './state/repoFormTypes';

// A repo script leaf. A run reference on the partner key shows as a note; typing here replaces it
// (the form's field table unsets the partner key in the same edit).
const props = defineProps<{
  id: string;
  testid: string;
  runRef?: RepoScriptRefValue;
  rows?: number;
  placeholder?: string;
  ariaLabel?: string;
}>();
const text = defineModel<string>({ default: '' });
</script>

<template>
  <div class="flex min-w-0 flex-1 flex-col gap-1">
    <Textarea
      v-if="props.rows"
      :id="id"
      :model-value="text"
      :rows="props.rows"
      class="field-sizing-fixed min-h-32 resize-y font-data leading-normal"
      :data-testid="testid"
      @update:model-value="(v) => (text = String(v))"
    />
    <Input
      v-else
      :id="id"
      :model-value="text"
      :placeholder="placeholder"
      :aria-label="ariaLabel"
      class="min-w-0 flex-1 font-data"
      :data-testid="testid"
      @update:model-value="(v) => (text = String(v))"
    />
    <FieldDescription v-if="runRef && !text" :data-testid="`${testid}-ref`">
      {{ t('space.repo.config.scriptRef.hint', { script: runRef.script ?? '' }) }}
    </FieldDescription>
  </div>
</template>
