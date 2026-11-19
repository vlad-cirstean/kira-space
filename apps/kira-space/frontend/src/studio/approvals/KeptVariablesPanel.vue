<script setup lang="ts">
import type { KeptUse } from '@shared/domain/kept';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useSettingsStore } from '../../state/settings';
import { keptSiteLabel } from '../kept/keptFormat';
import KeptVariableRow from './KeptVariableRow.vue';

// The kept variables a request reads, listed above its text so the user sees every one before
// answering. Generic over `KeptUse`: the DB approval bodies reuse it. `pairName` names the header,
// query or form entry a site points at.
const props = defineProps<{
  uses: readonly KeptUse[];
  pairName: (field: 'header' | 'query' | 'form', index: number) => string;
}>();

const settings = useSettingsStore();

function usedIn(u: KeptUse): string {
  return [...new Set(u.sites.map((s) => keptSiteLabel(s, props.pairName)))].join(', ');
}
</script>

<template>
  <section class="flex flex-col gap-1" data-testid="api-approval-kept-panel">
    <SectionHeading flush :label="t('studio.kept.panel.title.label')">
      <Button
        variant="link"
        size="inline"
        class="text-kira-sm font-normal"
        data-testid="api-approval-kept-manage"
        @click="settings.openSettingsAt('Studio MCP variables')"
      >
        {{ t('studio.kept.manage.label') }}
      </Button>
    </SectionHeading>
    <ul class="m-0 flex list-none flex-col gap-2 p-0">
      <li v-for="u in uses" :key="u.id" :data-testid="`api-approval-kept-${u.name}`">
        <KeptVariableRow :use="u" :used-in="usedIn(u)" />
      </li>
    </ul>
  </section>
</template>
