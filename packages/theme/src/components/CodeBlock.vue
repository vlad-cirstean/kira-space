<script setup lang="ts">
import { Card } from '@theme/components/ui/card';
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

// A block of literal text (a command, a payload, a config snippet) in the data font.
const props = defineProps<{ class?: HTMLAttributes['class'] }>();
</script>

<template>
  <Card
    data-slot="code-block"
    :class="cn('select-text font-data text-kira-sm whitespace-pre-wrap break-all', props.class)"
  >
    <slot />
  </Card>
</template>
