<script setup lang="ts">
import type { KeptEntry } from '@shared/domain/kept';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import RelativeTime from '@workbench/components/RelativeTime.vue';
import KeptValueView from '../../kept/KeptValueView.vue';
import { useKeptExpiry } from '../../kept/keptExpiry';
import { keptTypeLabel } from '../../kept/keptFormat';

// One live kept variable: what it is, who kept it, how long it has left, its value, Clear.
const props = defineProps<{ entry: KeptEntry }>();
defineEmits<{ clear: [] }>();

const expiry = useKeptExpiry(() => props.entry.expiresAt);
</script>

<template>
  <div :class="cn(rowVariants({ layout: 'double' }), 'flex-col whitespace-normal')">
    <div class="flex w-full min-w-0 items-center justify-between gap-1.5">
      <div class="flex min-w-0 items-baseline gap-1.5">
        <span class="font-data text-kira-md" data-testid="kept-entry-name">{{ entry.name }}</span>
        <span class="shrink-0 text-muted-foreground text-kira-sm">{{ keptTypeLabel(entry) }}</span>
        <span class="shrink-0 text-muted-foreground text-kira-sm">{{ entry.scopeLabel }}</span>
      </div>
      <TooltipIconButton
        :icon="ICON.delete"
        :label="t('studio.settings.kept.clear.aria', { name: entry.name })"
        data-testid="kept-entry-clear"
        @click="$emit('clear')"
      />
    </div>
    <div class="flex w-full min-w-0 items-baseline gap-1.5 text-muted-foreground text-kira-sm">
      <Badge v-if="entry.source.kind === 'user'" variant="info" data-testid="kept-entry-supplied">{{
        t('studio.kept.supplied.status')
      }}</Badge>
      <TruncatedText v-else class="font-data" :text="entry.source.request" />
      <span
        v-if="entry.origin"
        class="min-w-0 truncate font-data"
        data-testid="kept-entry-origin"
        >{{
          `${t('studio.kept.origin.label')} ${entry.origin}`
        }}</span
      >
      <span class="shrink-0" data-testid="kept-entry-age"
        ><RelativeTime :at="entry.createdAt" /></span
      >
      <span class="shrink-0" data-testid="kept-entry-expiry">{{ expiry }}</span>
    </div>
    <KeptValueView class="w-full" :entry="entry" />
  </div>
</template>
