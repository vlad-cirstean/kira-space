<script setup lang="ts">
import type { ReviewRemovedEntry } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@theme/components/ui/collapsible';
import { FieldDescription } from '@theme/components/ui/field';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';
import RelativeTime from '../../../hostview/RelativeTime.vue';
import type { ReviewWindowTarget } from '../wire';
import { useReviewRemoved } from './useReviewRemoved';

// Reviewed lines that left the branch since review: a reverted or deleted file, deleted lines, and
// lines a re-mark replaced. Each stays until dismissed.
const props = defineProps<{ target: ReviewWindowTarget }>();

const { removed, dismiss } = useReviewRemoved(() => props.target);
const entries = computed(() => removed.data.value ?? []);
const open = ref(true);

/** Text lines with their snapshot line numbers; the trailing newline adds no line. */
function numbered(entry: ReviewRemovedEntry): { n: number; text: string }[] {
  if (entry.text === null || entry.range === null) return [];
  const start = entry.range.start;
  return entry.text
    .replace(/\n$/, '')
    .split('\n')
    .map((text, i) => ({ n: start + i, text }));
}

const rows = computed(() => entries.value.map((entry) => ({ entry, lines: numbered(entry) })));

function isDismissing(id: number): boolean {
  return dismiss.isPending.value && dismiss.variables.value?.includes(id) === true;
}

function rangeLabel(entry: ReviewRemovedEntry): string {
  const r = entry.range;
  if (r === null) return t('space.ade.review.removed.wholeFile.label');
  return r.start === r.end
    ? t('space.ade.review.removed.line.label', { line: r.start })
    : t('space.ade.review.removed.lines.label', { start: r.start, end: r.end });
}
</script>

<template>
  <Collapsible
    v-if="rows.length > 0"
    v-model:open="open"
    class="flex min-h-0 flex-col"
    :aria-label="t('space.ade.review.removed.section.aria')"
    data-testid="ade-review-removed"
  >
    <CollapsibleTrigger
      class="flex shrink-0 items-center gap-1 px-2 py-1 text-left text-kira-sm font-medium text-muted-foreground"
      data-testid="ade-review-removed-toggle"
    >
      <CodiconIcon :name="open ? 'chevron-down' : 'chevron-right'" :size="12" />
      {{ t('space.ade.review.removed.header.label', { count: entries.length }) }}
    </CollapsibleTrigger>
    <CollapsibleContent class="flex min-h-0 flex-col gap-1 overflow-auto px-2 pb-2">
      <Alert
        v-if="dismiss.status.value === 'error'"
        variant="warn"
        data-testid="ade-review-removed-error"
      >
        <AlertDescription>{{ t('space.ade.review.removed.dismissFailed.body') }}</AlertDescription>
      </Alert>
      <Card v-for="{ entry, lines } in rows" :key="entry.id" data-testid="ade-review-removed-entry">
        <div class="flex items-center gap-1 text-kira-sm">
          <span class="min-w-0 flex-1 truncate font-medium" data-testid="ade-review-removed-path">{{
            entry.path
          }}</span>
          <Badge
            v-if="entry.origin === 'working'"
            variant="info"
            data-testid="ade-review-removed-origin"
          >
            {{ t('space.ade.changes.uncommitted.label') }}
          </Badge>
          <Tooltip>
            <TooltipTrigger as-child>
              <Button
                variant="toolbar"
                size="kira-icon"
                :aria-label="t('space.ade.review.removed.dismiss.aria', { path: entry.path })"
                :pending="isDismissing(entry.id)"
                data-testid="ade-review-removed-dismiss"
                @click="dismiss.mutate([entry.id])"
              >
                <CodiconIcon name="close" :size="12" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{{ t('space.ade.review.removed.dismiss.tooltip') }}</TooltipContent>
          </Tooltip>
        </div>
        <div class="flex gap-2 text-kira-sm text-muted-foreground">
          <span data-testid="ade-review-removed-reviewed"
            >{{ t('space.ade.review.removed.reviewed.label') }}
            <RelativeTime :at="entry.reviewedAt" /></span
          >
          <span data-testid="ade-review-removed-range">{{ rangeLabel(entry) }}</span>
        </div>
        <pre
          v-if="entry.text !== null"
          class="m-0 max-h-40 overflow-auto font-data text-kira-sm"
          data-testid="ade-review-removed-text"
        ><code><span
          v-for="line in lines"
          :key="line.n"
          class="flex gap-2 text-error"
        ><span class="w-8 shrink-0 select-none text-right text-muted-foreground">{{ line.n }}</span><span class="min-w-0 whitespace-pre-wrap break-all">{{ line.text }}</span></span></code></pre>
        <FieldDescription v-else>
          {{ t('space.ade.review.removed.notStored.hint') }}
        </FieldDescription>
        <FieldDescription v-if="entry.truncated" data-testid="ade-review-removed-truncated">
          {{ t('space.ade.review.removed.truncated.hint') }}
        </FieldDescription>
      </Card>
    </CollapsibleContent>
  </Collapsible>
</template>
