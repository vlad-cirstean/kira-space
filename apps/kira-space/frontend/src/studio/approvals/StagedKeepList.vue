<script setup lang="ts">
import type { KeptStaged } from '@shared/domain/kept';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { keptTypeLabel } from '../kept/keptFormat';
import { useDropStaged } from '../kept/keptQueries';
import TempVarRow from '../kept/TempVarRow.vue';

// The values a script sets, and the temporary values it deletes, once its held result is sent: not stored
// yet. A plain value shows its preview; a secret shows dots and Reveal, which reads it from the held result
// behind the OS gate. The x drops one: it is not set and the agent is told.
const props = defineProps<{ keep: readonly KeptStaged[]; testid: string; requestId: string }>();
const drop = useDropStaged(() => props.requestId);
</script>

<template>
  <section class="flex flex-col gap-1" :data-testid="testid">
    <SectionHeading flush :label="t('studio.kept.willKeep.label')" />
    <ul class="m-0 flex list-none flex-col gap-0.5 p-0">
      <li v-for="k in keep" :key="k.name" :data-testid="`${testid}-${k.name}`">
        <TempVarRow
          :name="k.name"
          :secret="k.secret"
          :deleted="k.deleted"
          :entry="{ ...k, id: '' }"
          :details="[keptTypeLabel(k)]"
          staged
          :staged-reveal="{ requestId, name: k.name }"
        >
          <template #actions>
            <TooltipIconButton
              :icon="ICON.close"
              :label="t('studio.kept.drop.tooltip')"
              :aria-label="t('common.action.remove.label')"
              :data-testid="`${testid}-${k.name}-drop`"
              :disabled="drop.isPending.value"
              @click="drop.mutate(k.name)"
            />
          </template>
        </TempVarRow>
      </li>
    </ul>
  </section>
</template>
