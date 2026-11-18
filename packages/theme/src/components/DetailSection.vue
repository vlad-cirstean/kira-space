<script setup lang="ts">
import { Card, CardAction, CardContent, CardHeader, CardTitle } from '@theme/components/ui/card';

// Titled section card (Card variant="section").
defineProps<{ title: string; flush?: boolean }>();
</script>

<template>
  <Card variant="section">
    <CardHeader>
      <CardTitle as="h3">{{ title }}</CardTitle>
      <CardAction class="normal-case tracking-normal"><slot name="actions" /></CardAction>
    </CardHeader>
    <CardContent :flush="flush" :class="flush ? undefined : 'flex flex-col gap-2'">
      <slot />
    </CardContent>
  </Card>
</template>
