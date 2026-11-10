<script setup lang="ts">
import Disclosure from '@theme/components/Disclosure.vue';
import { type MessageKey, t } from '@theme/i18n';

// What masking does and does not cover, kept out of the way: nobody needs it to use the tab.
const LIMITS: readonly MessageKey[] = [
  'studio.project.connectionDialog.privacy.limitsMatch.body',
  'studio.project.connectionDialog.privacy.limitsBestEffort.body',
  'studio.project.connectionDialog.privacy.limitsCheck.body',
  'studio.project.connectionDialog.privacy.limitsScope.body',
];
</script>

<template>
  <Disclosure
    :label="t('studio.project.connectionDialog.privacy.limits.label')"
    trigger-class="text-fg font-medium"
    content-class="pt-1.5"
    data-testid="connection-privacy-limits"
  >
    <ul
      class="flex list-disc flex-col gap-1 pl-4 text-kira-sm text-subtle"
      data-testid="connection-privacy-enforce-masking"
    >
      <li v-for="key in LIMITS" :key="key">{{ t(key) }}</li>
    </ul>
  </Disclosure>
</template>
