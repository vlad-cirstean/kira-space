<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { floatingSurface } from '@theme/lib/frames';

// A floating "work is waiting" toast: the host's text lines and a Show button.
defineProps<{ testid: string; showTestid: string }>();
const emit = defineEmits<{ show: [] }>();
</script>

<template>
  <div
    role="status"
    aria-live="polite"
    :class="[floatingSurface, 'flex items-center gap-3 p-3']"
    :data-testid="testid"
  >
    <div class="flex min-w-0 flex-col"><slot /></div>
    <Button variant="dialog-primary" size="kira-lg" :data-testid="showTestid" @click="emit('show')">
      {{ t('common.action.show.label') }}
    </Button>
  </div>
</template>
