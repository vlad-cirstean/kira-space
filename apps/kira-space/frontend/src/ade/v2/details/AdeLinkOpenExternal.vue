<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { control } from '../../../bridge/control';

// Opens a link row's url in the system browser through the shared openExternal path. Hidden without a url.
const props = defineProps<{ url: string }>();
</script>

<template>
  <TooltipIconButton
    v-if="props.url"
    icon="link-external"
    :label="t('space.ade.details.openExternal.tooltip')"
    class="shrink-0"
    data-testid="ade-details-open"
    @click="fireAndForget(control.openExternal(props.url), 'ade.details')"
  />
</template>
