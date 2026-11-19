<script setup lang="ts">
import { statusClass, statusHint } from '@shared/domain/http';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Badge } from '@theme/components/ui/badge';

// The one HTTP status chip: tone from the status class, the status meaning as its tooltip. Attrs
// (testid included) land on the Badge.
defineOptions({ inheritAttrs: false });
defineProps<{ status: number; statusText?: string }>();
</script>

<template>
  <TextTooltip :text="statusHint(status)">
    <Badge data :variant="statusClass(status)" v-bind="$attrs">{{ status }} {{ statusText }}</Badge>
  </TextTooltip>
</template>
