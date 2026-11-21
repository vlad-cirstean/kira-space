<script setup lang="ts">
import HighlightedText from '@theme/components/HighlightedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { KEPT_MARK_CLASS } from './keptHighlight';

// A kept variable's name in the same mark the request text carries, then its type and, when `secret` is
// given, whether it is secret or plain.
defineProps<{ name: string; type: string; secret?: boolean }>();
</script>

<template>
  <span class="inline-flex min-w-0 flex-wrap items-center gap-x-1 font-data"
    ><HighlightedText :parts="[{ text: name, match: true }]" :mark-class="KEPT_MARK_CLASS" />
    <span class="text-muted-foreground"> · {{ type }}</span>
    <Badge
      v-if="secret !== undefined"
      :variant="secret ? 'warn' : 'info'"
      :data-testid="secret ? 'kept-kind-secret' : 'kept-kind-plain'"
      >{{
        t(secret ? 'common.secret.label' : 'studio.kept.kind.plain.label')
      }}</Badge
    ></span
  >
</template>
