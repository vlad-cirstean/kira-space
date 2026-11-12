<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SplitButton from '@theme/components/SplitButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import {
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { computed, toRef } from 'vue';
import { useChooseShotLocation, useResetShotLocation, useShotLocation } from './queries';
import { useProofWindowStore } from './state';
import type { ProofTab } from './tabHost';
import { useProofScreenshot } from './useProofScreenshot';

// Primary action saves every tab into a new folder under the save location; the menu copies the
// active tab and shows or changes that location.
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

const tooltip = computed(() =>
  location.data.value
    ? t('studio.proof.shot.saveAll.tooltip', { path: location.data.value.path })
    : t('studio.proof.shot.saveAllNoPath.tooltip'),
);

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
    :disabled="tabs.length === 0"
    data-testid="proof-shot"
    @click="shot.saveAll()"
  >
    <CodiconIcon name="device-camera" :size="13" />
    {{ t('studio.proof.shot.saveAll.label') }}
    <template #menu>
      <DropdownMenuContent align="end">
        <DropdownMenuItem data-testid="proof-shot-copy" @select="shot.copy()">
          {{ t('studio.proof.shot.copy.label') }}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
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
        <DropdownMenuItem
          data-testid="proof-shot-location-choose"
          @select="choose.mutate(undefined, { onError: fail })"
        >
          {{ t('studio.proof.shot.location.choose.label') }}
        </DropdownMenuItem>
        <DropdownMenuItem
          v-if="location.data.value?.custom"
          data-testid="proof-shot-location-reset"
          @select="reset.mutate(undefined, { onError: fail })"
        >
          {{ t('studio.proof.shot.location.reset.label') }}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </template>
  </SplitButton>
</template>
