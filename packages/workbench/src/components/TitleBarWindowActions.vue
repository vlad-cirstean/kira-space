<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
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

// §8.3: aria-pressed plus this state-naming tooltip carry what a missing coffee-off glyph would
// have — @vscode/codicons ships no such glyph, so the button can't also swap its icon the way the
// Connections/Operations toggles do.
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
    data-testid="toggle-keep-awake"
    @click="emit('toggleKeepAwake')"
  />
  <TextTooltip :text="t('workbench.titleBar.newWindow.label')" shortcut="window.new">
    <Button
      variant="title"
      size="title-labelled"
      data-testid="new-window"
      @click="emit('newWindow')"
    >
      <CodiconIcon name="empty-window" :size="16" />
      <span>{{ t('workbench.titleBar.newWindow.label') }}</span>
    </Button>
  </TextTooltip>
</template>
