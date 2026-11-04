<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';

// Sticky bar while scrolled into the open history region.
const emit = defineEmits<{ goToDate: [iso: string]; hide: []; current: [] }>();

function onDate(e: Event): void {
  const v = (e.target as HTMLInputElement).value;
  if (v) emit('goToDate', v);
}
</script>

<template>
  <div
    class="flex items-center gap-2 bg-elevated px-3 py-1.5 text-kira-md"
    data-testid="ade-history-bar"
  >
    <span class="font-medium">{{ t('common.module.history.label') }}</span>
    <Label for="ade-go-to-date" class="text-muted-foreground">{{
      t('space.ade.plan.history.goTo.label')
    }}</Label>
    <Input
      id="ade-go-to-date"
      type="date"
      size="kira-lg"
      class="w-36 scheme-dark"
      data-testid="ade-go-to-date"
      @input="onDate"
    />
    <span class="flex-1" />
    <Button variant="dialog" size="kira-lg" data-testid="ade-hide-history" @click="emit('hide')">
      {{ t('space.ade.plan.history.hide.label') }}
    </Button>
    <Button
      variant="dialog-primary"
      size="kira-lg"
      data-testid="ade-current-work"
      @click="emit('current')"
    >
      {{ t('space.ade.plan.history.current.label') }}
    </Button>
  </div>
</template>
