<script setup lang="ts">
import type { KeptStaged } from '@shared/domain/kept';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import KeptNameMark from '../kept/KeptNameMark.vue';
import KeptValueView from '../kept/KeptValueView.vue';
import { keptTypeLabel } from '../kept/keptFormat';
import { useDropStaged } from '../kept/keptQueries';

// The values a script saves, and the saved values it deletes, once its held result is sent: not stored yet. A plain value shows its
// preview; a secret shows dots and Reveal, which reads it from the held result behind the OS gate.
const props = defineProps<{ keep: readonly KeptStaged[]; testid: string; requestId: string }>();
const drop = useDropStaged(() => props.requestId);
</script>

<template>
  <section class="flex flex-col gap-1" :data-testid="testid">
    <SectionHeading flush :label="t('studio.kept.willKeep.label')" />
    <ul class="m-0 flex list-none flex-col gap-2 p-0">
      <li
        v-for="k in keep"
        :key="k.name"
        class="flex flex-col gap-0.5"
        :data-testid="`${testid}-${k.name}`"
      >
        <div class="flex items-center justify-between gap-2">
          <KeptNameMark
            :class="k.deleted && 'line-through'"
            :name="k.name"
            :type="keptTypeLabel(k)"
            :secret="k.secret"
          />
          <TooltipIconButton
            :icon="ICON.close"
            :label="t('studio.kept.drop.tooltip')"
            :aria-label="t('common.action.remove.label')"
            :data-testid="`${testid}-${k.name}-drop`"
            :disabled="drop.isPending.value"
            @click="drop.mutate(k.name)"
          />
        </div>
        <span
          v-if="k.deleted"
          class="text-kira-sm text-muted-foreground"
          :data-testid="`${testid}-${k.name}-deleted`"
          >{{
            t('studio.kept.willKeep.deleted.status')
          }}</span
        >
        <KeptValueView
          v-else
          :entry="{ ...k, id: '' }"
          staged
          :staged-reveal="{ requestId, name: k.name }"
        />
      </li>
    </ul>
  </section>
</template>
