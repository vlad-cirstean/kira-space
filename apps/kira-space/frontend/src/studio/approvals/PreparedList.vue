<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { t } from '@theme/i18n';
import type { PreparedValue } from './preparedValues';

// The values a prepare script set, one row each, above the request text that marks them.
defineProps<{ rows: readonly PreparedValue[]; testid: string }>();
</script>

<template>
  <section class="flex flex-col gap-1" :data-testid="testid">
    <SectionHeading flush :label="t('studio.dbMcp.approval.api.scriptChanged.label')" />
    <ul class="m-0 flex list-none flex-col p-0 font-data text-kira-md">
      <li
        v-for="r in rows"
        :key="r.key"
        class="flex min-w-0 gap-2"
        :data-testid="`${testid}-${r.key}`"
      >
        <span class="shrink-0 text-syntax-property">{{ r.key }}</span>
        <span class="flex min-w-0 flex-1 text-syntax-string"
          ><TruncatedText :text="r.value" /></span
        >
      </li>
    </ul>
  </section>
</template>
