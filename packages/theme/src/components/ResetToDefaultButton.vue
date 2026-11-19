<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';

// The reset-to-default button of a settings field. Attrs (testid, `@click`, class) fall through.
withDefaults(defineProps<{ atDefault?: boolean }>(), { atDefault: false });
</script>

<template>
  <TooltipIconButton
    :icon="ICON.reset"
    :label="t('common.action.resetToDefault.label')"
    :disabled-trigger="atDefault"
    :disabled="atDefault"
  />
</template>
