<script setup lang="ts">
// Screen-reader copy of a menu row's hint: the row points `aria-describedby` at `id`. The visible
// hint is a hover tooltip, which keyboard and screen-reader users never get on a disabled row.
import { plainText, type TextPart } from '@theme/varText';

defineProps<{ id: string; testId: string; hint: string | readonly TextPart[] }>();
</script>

<template>
  <span :id="id" :data-testid="testId" class="sr-only">{{
    plainText(typeof hint === 'string' ? [hint] : hint)
  }}</span>
</template>
