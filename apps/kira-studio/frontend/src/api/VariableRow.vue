<script setup lang="ts">
import type { ApiVariable } from '@shared/domain/variables';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Popover, PopoverAnchor } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { ref, watch } from 'vue';
import { useVariableSetStore } from './state/variables';
import VariableHistoryMenu from './VariableHistoryMenu.vue';

const variableSetStore = useVariableSetStore();

// One read-only row (P473: the file is the editor): name, value (masked for a secret until
// revealed), description, a history button/popover.
const props = defineProps<{
  row: ApiVariable;
  /** The revealed plaintext of a secret row; undefined until revealed. */
  revealed?: string;
}>();

const emit = defineEmits<{
  /** Not yet revealed: the eye IS the reveal action. */
  reveal: [];
  /** The parent knows the tab/scope/owner this row's history popover needs to open against. */
  history: [];
}>();

// Once revealed, the eye is a free client-side mask toggle: no second round trip, no second prompt.
const visible = ref(false);
watch(
  () => props.revealed,
  (next, previous) => {
    if (previous === undefined && next !== undefined) visible.value = true;
  },
);

const notYetRevealed = () => props.row.isSecret && props.revealed === undefined;
const shown = () => (props.row.isSecret ? (props.revealed ?? '') : props.row.value);
const masked = () => props.row.isSecret && (notYetRevealed() || !visible.value);

function valueToggleLabel(): string {
  if (notYetRevealed()) return t('common.action.reveal.label');
  return visible.value ? t('common.hideValue.label') : t('studio.api.variables.showValue.label');
}

function onEyeClick(): void {
  if (notYetRevealed()) emit('reveal');
  else visible.value = !visible.value;
}

const showHistory = ref(false);
// PopoverAnchor's `:reference` takes the trigger's real DOM node directly (the established `.$el`
// idiom, e.g. ConsoleView.vue's savedMenuTriggerEl).
const historyAnchorRef = ref<{ $el: HTMLElement } | null>(null);
function onHistoryClick(): void {
  showHistory.value = true;
  emit('history');
}
function onHistoryOpenChange(open: boolean): void {
  if (open) return;
  showHistory.value = false;
  variableSetStore.closeHistoryMenu();
}
</script>

<template>
  <div
    class="grid items-center gap-1 px-1.5 py-1 grid-cols-[1.2fr_2fr_1.5fr_auto] variable-row"
    data-testid="variable-row"
    :data-id="row.id"
  >
    <span class="min-w-0 name-cell font-data text-kira-md" data-testid="variable-name"
      ><TruncatedText :text="row.name" /></span
    >
    <div class="flex min-w-0 items-center gap-1 value-cell">
      <!-- tracking-widest (0.1em) is the widest step Tailwind's default scale has. -->
      <span
        v-if="masked()"
        class="flex-1 text-subtle tracking-widest"
        data-testid="variable-value-masked"
        >••••••••</span
      >
      <span v-else class="min-w-0 flex-1 font-data text-kira-md" data-testid="variable-value"
        ><TruncatedText :text="shown()" /></span
      >
      <TooltipIconButton
        v-if="row.isSecret"
        icon="eye"
        :label="valueToggleLabel()"
        :aria-label="t('common.action.reveal.label')"
        :pressed="visible"
        data-testid="variable-reveal"
        @click="onEyeClick"
      />
    </div>
    <span
      class="min-w-0 description-cell text-kira-md text-subtle"
      data-testid="variable-description"
      ><TruncatedText :text="row.description" /></span
    >
    <Popover
      :open="showHistory && variableSetStore.variableId === row.id"
      @update:open="onHistoryOpenChange"
    >
      <div class="relative flex">
        <TooltipIconButton
          :icon="ICON.history"
          :label="t('common.module.history.label')"
          ref="historyAnchorRef"
          data-testid="variable-history"
          @click="onHistoryClick"
        />
        <PopoverAnchor :reference="(historyAnchorRef?.$el as HTMLElement) ?? undefined" />
      </div>
      <VariableHistoryMenu v-if="variableSetStore.variableId === row.id" />
    </Popover>
  </div>
</template>
