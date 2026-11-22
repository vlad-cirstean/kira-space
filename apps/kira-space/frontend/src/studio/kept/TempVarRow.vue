<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import { ref } from 'vue';
import KeptFullValue from './KeptFullValue.vue';
import KeptNameMark from './KeptNameMark.vue';
import KeptValueView from './KeptValueView.vue';
import { useKeptExpiry } from './keptExpiry';

// One temporary variable on one line, shared by the approval card, the review box and Settings: the name
// (its tooltip carries the source details), a Plain or Secret badge, the value, the time left and the
// row's actions. A stored value whose preview was cut opens whole under the line. The countdown is
// display only; the backend decides expiry. `staged` is a value not stored yet: it has no expiry.
const props = defineProps<{
  name: string;
  secret: boolean;
  deleted?: boolean;
  entry: { id: string; type: string; secret: boolean; preview: string; previewTruncated: boolean };
  expiresAt?: string;
  details: readonly string[];
  staged?: boolean;
  stagedReveal?: { requestId: string; name: string };
}>();

const expiry = useKeptExpiry(() => props.expiresAt ?? '');
const open = ref(false);
</script>

<template>
  <div class="flex min-w-0 flex-col">
    <div class="flex min-h-control min-w-0 items-center gap-1.5">
      <Tooltip>
        <TooltipTrigger as-child>
          <button
            type="button"
            :class="
              cn(
                'shrink-0 cursor-default rounded-kira-sm bg-transparent p-0 text-left text-kira-md focus-visible:focus-ring',
                deleted && 'line-through',
              )
            "
            data-testid="kept-name"
          >
            <KeptNameMark :name="name" />
          </button>
        </TooltipTrigger>
        <TooltipContent v-if="details.length" data-testid="kept-details"
          ><span v-for="d in details" :key="d" class="block">{{ d }}</span></TooltipContent
        >
      </Tooltip>
      <Badge
        :variant="secret ? 'warn' : 'info'"
        :data-testid="secret ? 'kept-kind-secret' : 'kept-kind-plain'"
        >{{
          t(secret ? 'common.secret.label' : 'studio.kept.kind.plain.label')
        }}</Badge
      >
      <span
        v-if="deleted"
        class="min-w-0 flex-1 truncate text-kira-sm text-muted-foreground"
        data-testid="kept-deleted"
        >{{
          t('studio.kept.willKeep.deleted.status')
        }}</span
      >
      <KeptValueView
        v-else
        v-model:open="open"
        class="min-w-0 flex-1"
        :entry="entry"
        :staged="staged"
        :staged-reveal="stagedReveal"
      />
      <span
        v-if="expiresAt"
        class="shrink-0 text-kira-sm text-muted-foreground tabular-nums"
        data-testid="kept-expiry"
        >{{
          expiry
        }}</span
      >
      <slot name="actions" />
    </div>
    <KeptFullValue v-if="open && !deleted && entry.id" :id="entry.id" />
  </div>
</template>
