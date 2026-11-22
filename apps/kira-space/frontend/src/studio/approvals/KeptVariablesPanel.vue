<script setup lang="ts">
import type { KeptUse } from '@shared/domain/kept';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useSettingsStore } from '../../state/settings';
import { keptDetails, keptSiteLabel } from '../kept/keptFormat';
import { useClearKept } from '../kept/keptQueries';
import TempVarRow from '../kept/TempVarRow.vue';

// The temporary variables a request reads, listed above its text so the user sees every one before
// answering, one line each. Generic over `KeptUse`: the DB and kube approval bodies reuse it. `pairName`
// names the header, query or form entry a site points at. The x deletes the stored value.
const props = withDefaults(
  defineProps<{
    uses: readonly KeptUse[];
    pairName: (field: 'header' | 'query' | 'form', index: number) => string;
    testidPrefix?: string;
  }>(),
  { testidPrefix: 'api-approval' },
);

const settings = useSettingsStore();
const clear = useClearKept();

function usedIn(u: KeptUse): string {
  return [...new Set(u.sites.map((s) => keptSiteLabel(s, props.pairName)))].join(', ');
}
</script>

<template>
  <section class="flex flex-col gap-1" :data-testid="`${testidPrefix}-kept-panel`">
    <SectionHeading flush :label="t('common.temporaryVariables.label')">
      <Button
        variant="link"
        size="inline"
        class="text-kira-sm font-normal"
        :data-testid="`${testidPrefix}-kept-manage`"
        @click="settings.openSettingsAt('Studio MCP variables')"
      >
        {{ t('studio.kept.manage.label') }}
      </Button>
    </SectionHeading>
    <ul class="m-0 flex list-none flex-col gap-0.5 p-0">
      <li v-for="u in uses" :key="u.id || u.name" :data-testid="`${testidPrefix}-kept-${u.name}`">
        <TempVarRow
          :name="u.name"
          :secret="u.secret"
          :entry="u"
          :expires-at="u.expiresAt"
          :details="keptDetails(u, { usedIn: usedIn(u) })"
        >
          <template #actions>
            <TooltipIconButton
              v-if="u.id"
              :icon="ICON.close"
              :label="t('studio.kept.clear.tooltip')"
              :aria-label="t('common.action.delete.label')"
              :data-testid="`${testidPrefix}-kept-${u.name}-clear`"
              :disabled="clear.isPending.value"
              @click="clear.mutate([u.id])"
            />
          </template>
        </TempVarRow>
      </li>
    </ul>
  </section>
</template>
