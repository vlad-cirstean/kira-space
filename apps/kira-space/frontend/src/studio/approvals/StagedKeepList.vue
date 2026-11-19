<script setup lang="ts">
import type { KeptStaged } from '@shared/domain/kept';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { t } from '@theme/i18n';
import KeptNameMark from '../kept/KeptNameMark.vue';
import KeptValueView from '../kept/KeptValueView.vue';
import { keptTypeLabel } from '../kept/keptFormat';

// The values a script keeps once its held result is sent: not stored yet, previews only.
defineProps<{ keep: readonly KeptStaged[]; testid: string }>();
</script>

<template>
  <section class="flex flex-col gap-1" :data-testid="testid">
    <SectionHeading flush :label="t('studio.kept.willKeep.label')" />
    <ul class="m-0 flex list-none flex-col gap-2 p-0">
      <li v-for="k in keep" :key="k.name" :data-testid="`${testid}-${k.name}`">
        <KeptNameMark :name="k.name" :type="keptTypeLabel(k)" />
        <KeptValueView :entry="{ ...k, id: '' }" staged />
      </li>
    </ul>
  </section>
</template>
