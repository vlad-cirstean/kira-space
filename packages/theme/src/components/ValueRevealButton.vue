<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';

// The one show/hide toggle for a masked value: "Show value" / "Hide value", eye flips, pressed when
// shown. `label` overrides the tooltip only (a not-revealable reason); the accessible name stays.
// Every other attribute (testid, click, disabled, disabled-trigger, type) lands on the button.
defineOptions({ inheritAttrs: false });
defineProps<{ shown: boolean; label?: string }>();
</script>

<template>
  <TooltipIconButton
    :icon="shown ? 'eye-closed' : 'eye'"
    :label="label ?? (shown ? t('common.hideValue.label') : t('common.secret.show.label'))"
    :aria-label="shown ? t('common.hideValue.label') : t('common.secret.show.label')"
    :pressed="shown"
    v-bind="$attrs"
  />
</template>
