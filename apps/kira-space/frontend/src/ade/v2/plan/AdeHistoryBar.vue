<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
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
    <Field orientation="horizontal" class="w-auto">
      <FieldLabel for="ade-go-to-date">{{ t('space.ade.plan.history.goTo.label') }}</FieldLabel>
      <Input
        id="ade-go-to-date"
        type="date"
        class="w-40 scheme-dark"
        data-testid="ade-go-to-date"
        @input="onDate"
      />
    </Field>
    <span class="flex-1" />
    <Button variant="toolbar" size="kira" data-testid="ade-hide-history" @click="emit('hide')"
      ><CodiconIcon name="eye-closed" :size="13" />{{ t('space.ade.plan.history.hide.label') }}
    </Button>
    <Button variant="toolbar" size="kira" data-testid="ade-current-work" @click="emit('current')"
      ><CodiconIcon name="arrow-right" :size="13" />{{ t('space.ade.plan.history.current.label') }}
    </Button>
  </div>
</template>
