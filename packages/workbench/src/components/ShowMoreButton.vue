<script setup lang="ts">
// "Show more (M remaining)" for any list capped by `useCappedList`; `@click` reaches the caller's
// own `showMore` through the single-root attrs fallthrough.
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';

// `step` stays for callers; the label states the remaining count only.
defineProps<{ hiddenCount: number; step: number }>();
</script>

<template>
  <Button
    v-if="hiddenCount > 0"
    variant="link"
    size="inline"
    class="w-full justify-start px-2 py-0.5 h-auto text-muted-foreground"
  >
    {{ t('workbench.showMore.remaining.label', { count: hiddenCount }) }}
  </Button>
</template>
