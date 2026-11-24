<script setup lang="ts">
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import type { KeepAwakeStatus } from '../bridge/createCoreControl';

// The keep-awake toggle and "New window" button, hoisted verbatim from Kira Studio's own
// TitleBar.vue (same data-testids, same DOM order — Kira Space's own copy, workbench.spec.ts's own
// DOM-order assertion covers both apps' rightmost two title-bar buttons). The click/status logic
// (the actual keepAwakeStore/control.windowsOpenNew calls) stays app-side: this component only
// renders and emits, since each app's own store/control instance differs.
const props = defineProps<{ keepAwake: KeepAwakeStatus }>();
const emit = defineEmits<{ toggleKeepAwake: []; newWindow: [] }>();

// §8.3: aria-pressed plus this state-naming tooltip carry the state for assistive tech.
// @vscode/codicons has no filled coffee glyph, so on is the same icon in the blue on-state colour
// (P754) plus a decorative dot; the button's pressed background stays the title variant's.
const keepAwakeTooltip = computed(() => {
  if (props.keepAwake.error)
    return t('workbench.titleBar.keepAwakeFailed.error', { error: props.keepAwake.error });
  return props.keepAwake.manual
    ? t('workbench.titleBar.allowSleep.tooltip')
    : t('workbench.titleBar.keepAwake.tooltip');
});
</script>

<template>
  <TooltipIconButton
    v-if="keepAwake.supported"
    icon="coffee"
    :label="keepAwakeTooltip"
    :aria-label="t('workbench.titleBar.keepAwake.tooltip')"
    :icon-size="16"
    variant="title"
    size="title"
    :aria-pressed="keepAwake.manual"
    class="relative aria-pressed:text-toggle-on-edge"
    data-testid="toggle-keep-awake"
    @click="emit('toggleKeepAwake')"
  >
    <template #badge>
      <StatusDot
        v-if="keepAwake.manual"
        aria-hidden="true"
        class="pointer-events-none absolute top-0.5 right-0.5 size-1.5!"
        tone="info"
        data-testid="keep-awake-dot"
      />
    </template>
  </TooltipIconButton>
  <TooltipIconButton
    icon="empty-window"
    :label="t('workbench.titleBar.newWindow.label')"
    shortcut="window.new"
    :icon-size="16"
    variant="title"
    size="title"
    data-testid="new-window"
    @click="emit('newWindow')"
  />
</template>
