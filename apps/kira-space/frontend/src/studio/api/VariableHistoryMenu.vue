<script setup lang="ts">
import type { ApiVariableHistoryEntry } from '@shared/domain/variables';
import InlineState from '@theme/components/InlineState.vue';
import MaskedValue from '@theme/components/MaskedValue.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { PopoverContent, PopoverList } from '@theme/components/ui/popover';
import ValueRevealButton from '@theme/components/ValueRevealButton.vue';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import RelativeTime from '@workbench/components/RelativeTime.vue';
import { reactive } from 'vue';
import { useVariableSetStore } from './state/variables';

const variableSetStore = useVariableSetStore();

// The per-row history popover — a PopoverContent anchored to the row's own history button
// via VariableRow.vue's own PopoverAnchor (a plain content component; VariableRow.vue's Popover
// owns open state and the closeHistoryMenu() side effect for every dismissal). Each entry's
// relative recorded time, its value (masked for a secret, with its own gated eye — a secret's old
// value is exactly as sensitive as its current one). Read-only: a file edit is the way back.

// Client-side re-mask of an already revealed entry: no IPC, no prompt.
const hidden = reactive(new Set<string>());

function displayValue(entry: ApiVariableHistoryEntry): string {
  return entry.isSecret ? (variableSetStore.revealedHistoryValues[entry.id] ?? '') : entry.value;
}
function isMasked(entry: ApiVariableHistoryEntry): boolean {
  return (
    entry.isSecret &&
    (variableSetStore.revealedHistoryValues[entry.id] === undefined || hidden.has(entry.id))
  );
}
function canToggle(entry: ApiVariableHistoryEntry): boolean {
  return entry.isSecret;
}

function onEye(entry: ApiVariableHistoryEntry): void {
  if (variableSetStore.revealedHistoryValues[entry.id] === undefined) {
    hidden.delete(entry.id);
    void variableSetStore.revealHistoryEntry(entry.id);
    return;
  }
  if (!hidden.delete(entry.id)) hidden.add(entry.id);
}
</script>

<template>
  <PopoverContent align="start" flush class="w-80" data-testid="variable-history">
    <PopoverList class="max-h-80 overflow-auto">
      <InlineState
        v-if="variableSetStore.entries.length === 0"
        kind="empty"
        data-testid="variable-history-empty"
        >{{
          t('studio.api.variableHistory.empty')
        }}</InlineState
      >
      <div
        v-for="entry in variableSetStore.entries"
        :key="entry.id"
        :class="cn(rowVariants(), 'h-auto py-1')"
        data-testid="variable-history-entry"
      >
        <div class="flex min-w-0 flex-1 flex-col gap-0.5">
          <span class="text-subtle text-kira-sm"><RelativeTime :at="entry.recordedAt" /></span>
          <MaskedValue
            v-if="isMasked(entry)"
            class="truncate"
            data-testid="variable-history-masked"
          />
          <span v-else class="min-w-0 font-data" data-testid="variable-history-value"
            ><TruncatedText :text="displayValue(entry)" /></span
          >
        </div>
        <ValueRevealButton
          v-if="canToggle(entry)"
          :shown="!isMasked(entry)"
          data-testid="variable-history-reveal"
          @click="onEye(entry)"
        />
      </div>
    </PopoverList>
  </PopoverContent>
</template>
