<script setup lang="ts">
import type { KeptStaged } from '@shared/domain/kept';
import HighlightedText from '@theme/components/HighlightedText.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { t } from '@theme/i18n';
import KeptValueView from '../kept/KeptValueView.vue';
import { keptTypeLabel } from '../kept/keptFormat';
import { KEPT_MARK_CLASS } from '../kept/keptHighlight';

// The values a script keeps once its held result is sent: not stored yet, previews only.
defineProps<{ keep: readonly KeptStaged[]; testid: string }>();
</script>

<template>
  <section class="flex flex-col gap-1" :data-testid="testid">
    <SectionHeading flush :label="t('studio.kept.willKeep.label')" />
    <ul class="m-0 flex list-none flex-col gap-2 p-0">
      <li v-for="k in keep" :key="k.name" :data-testid="`${testid}-${k.name}`">
        <HighlightedText
          :parts="[{ text: k.name, match: true }]"
          :mark-class="`font-data ${KEPT_MARK_CLASS}`"
        />
        <span class="text-kira-sm text-muted-foreground"> · {{ keptTypeLabel(k) }}</span>
        <KeptValueView :entry="{ ...k, id: '' }" staged />
      </li>
    </ul>
  </section>
</template>
