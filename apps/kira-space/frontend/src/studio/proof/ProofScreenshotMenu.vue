<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SplitButton from '@theme/components/SplitButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { DropdownMenuContent, DropdownMenuLabel } from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import MenuItems from '@workbench/components/MenuItems.vue';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed, toRef } from 'vue';
import {
  useChooseShotLocation,
  useResetShotLocation,
  useShotClipboard,
  useShotLocation,
} from './queries';
import { useProofWindowStore } from './state';
import type { ProofTab } from './tabHost';
import { useProofScreenshot } from './useProofScreenshot';

// Primary action copies every tab as separate clipboard images where the system can, else saves
// every tab into a new folder under the save location. The menu copies the active tab and shows or
// changes that location.
const props = defineProps<{
  target: HTMLElement | null;
  tabs: ProofTab[];
  sessionName: string;
}>();
const store = useProofWindowStore();
const shot = useProofScreenshot({
  target: toRef(props, 'target'),
  tabs: toRef(props, 'tabs'),
  sessionName: toRef(props, 'sessionName'),
});
const location = useShotLocation();
const choose = useChooseShotLocation();
const reset = useResetShotLocation();

const clipboard = useShotClipboard();
// An unanswered or failed capability query means save.
const primary = computed(() => (clipboard.data.value?.multiImage === true ? 'copyAll' : 'saveAll'));

const tooltip = computed(() => {
  if (primary.value === 'copyAll') return t('studio.proof.shot.copyAll.tooltip');
  return location.data.value
    ? t('studio.proof.shot.saveAll.tooltip', { path: location.data.value.path })
    : t('studio.proof.shot.saveAllNoPath.tooltip');
});

const copyItems = computed<MenuItem[]>(() => [
  {
    type: 'item',
    id: 'copy',
    testId: 'proof-shot-copy',
    label: t('studio.proof.shot.copy.label'),
    icon: 'copy',
    run: () => shot.copy(),
  },
  ...(primary.value === 'copyAll'
    ? [
        {
          type: 'item' as const,
          id: 'save-all',
          testId: 'proof-shot-save-all',
          label: t('studio.proof.shot.saveAll.label'),
          icon: 'save-all',
          run: () => shot.saveAll(),
        },
      ]
    : []),
  { type: 'separator' },
]);
const locationItems = computed<MenuItem[]>(() => [
  {
    type: 'item',
    id: 'location-choose',
    testId: 'proof-shot-location-choose',
    label: t('studio.proof.shot.location.choose.label'),
    icon: 'folder',
    run: () => choose.mutate(undefined, { onError: fail }),
  },
  ...(location.data.value?.custom
    ? [
        {
          type: 'item' as const,
          id: 'location-reset',
          testId: 'proof-shot-location-reset',
          label: t('studio.proof.shot.location.reset.label'),
          icon: 'discard',
          run: () => reset.mutate(undefined, { onError: fail }),
        },
      ]
    : []),
]);

function fail(e: unknown): void {
  store.setShotResult({ kind: 'error', message: errorText(e) });
}
</script>

<template>
  <SplitButton
    :tooltip="tooltip"
    :chevron-label="t('studio.proof.shot.more.aria')"
    chevron-testid="proof-shot-more"
    :pending="shot.busy.value"
    :disabled="tabs.length === 0 || clipboard.isPending.value"
    :data-mode="primary"
    data-testid="proof-shot"
    @click="shot[primary]()"
  >
    <CodiconIcon :name="primary === 'copyAll' ? 'copy' : 'device-camera'" :size="13" />
    {{
      primary === 'copyAll'
        ? t('studio.proof.shot.copyAll.label')
        : t('studio.proof.shot.saveAll.label')
    }}
    <template #menu>
      <DropdownMenuContent align="end">
        <MenuItems :items="copyItems" />
        <!-- rich block: MenuItem has no multi-line label kind -->
        <DropdownMenuLabel
          v-if="location.data.value"
          class="flex h-auto max-w-80 flex-col items-stretch justify-start gap-0.5 py-1 font-normal normal-case tracking-normal"
          data-testid="proof-shot-location"
        >
          <span class="text-muted-foreground">{{ t('studio.api.saveRequest.target.label') }}</span>
          <span class="text-fg"
            ><TruncatedText :text="location.data.value.path" split="path" /></span
          >
          <span v-if="!location.data.value.custom" class="text-muted-foreground">{{
            t('common.state.default.option')
          }}</span>
          <span v-else-if="!location.data.value.exists" class="text-error">{{
            t('studio.proof.shot.location.missing.status')
          }}</span>
        </DropdownMenuLabel>
        <MenuItems :items="locationItems" />
      </DropdownMenuContent>
    </template>
  </SplitButton>
</template>
