<script setup lang="ts">
import type { KeptStaged } from '@shared/domain/kept';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { t } from '@theme/i18n';
import KeptNameMark from '../kept/KeptNameMark.vue';
import KeptValueView from '../kept/KeptValueView.vue';
import { keptTypeLabel } from '../kept/keptFormat';

// The values a script keeps once its held result is sent: not stored yet. A plain value shows its
// preview; a secret shows dots and Reveal, which reads it from the held result behind the OS gate.
defineProps<{ keep: readonly KeptStaged[]; testid: string; requestId: string }>();
</script>

<template>
  <section class="flex flex-col gap-1" :data-testid="testid">
    <SectionHeading flush :label="t('studio.kept.willKeep.label')" />
    <ul class="m-0 flex list-none flex-col gap-2 p-0">
      <li v-for="k in keep" :key="k.name" :data-testid="`${testid}-${k.name}`">
        <KeptNameMark :name="k.name" :type="keptTypeLabel(k)" :secret="k.secret" />
        <KeptValueView
          :entry="{ ...k, id: '' }"
          staged
          :staged-reveal="{ requestId, name: k.name }"
        />
      </li>
    </ul>
  </section>
</template>
