<script setup lang="ts">
import TruncatedText from '@theme/components/TruncatedText.vue';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import CopyIconButton from './CopyIconButton.vue';

// An id, sha, branch, ref or path shown as a value: always `font-data`, truncated with a tooltip
// for the full text, optionally shortened and copyable.
const props = defineProps<{
  value: string;
  /** `path` keeps the file name visible and truncates the directory instead; ignores `short`. */
  split?: 'path';
  /** Shortens the shown text (`shortSha`, a docker short id); the tooltip and copy keep `value`. */
  short?: (value: string) => string;
  /** Adds a trailing copy button that copies `value`. */
  copyable?: boolean;
}>();

const shown = computed(() => (props.short ? props.short(props.value) : props.value));
</script>

<template>
  <span class="inline-flex min-w-0 items-center gap-1 font-data">
    <TruncatedText :text="value" :split="split"><slot>{{ shown }}</slot></TruncatedText>
    <CopyIconButton
      v-if="copyable"
      :text="value"
      :label="t('workbench.identifier.copy.label')"
      class="shrink-0"
    />
  </span>
</template>
