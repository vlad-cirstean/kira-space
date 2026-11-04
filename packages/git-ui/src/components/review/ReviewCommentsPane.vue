<script setup lang="ts">
import type { LineRange, ReviewComment } from '@kira/git-ipc';
import AttributeTooltip from '@theme/components/AttributeTooltip.vue';
/**
 * The review sidebar's Comments pane, and the only new component this phase adds to
 * `packages/git-ui`: the centralized list, the copy-for-AI action and clear-all. The webview never
 * adds a comment — that gesture lives entirely in the editor's gutter — so this pane is
 * read, remove and clear-all only.
 *
 * Comments render in the server's own order (path, then line, then created_at, then id) —
 * this component groups consecutive same-path comments into one visual section without
 * re-sorting.
 */
import InlineState from '@theme/components/InlineState.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, useTemplateRef } from 'vue';
import { shaLabel } from '../../lib/text.ts';
import type { ReviewCommentsState } from '../../state/reviewComments.ts';

const props = defineProps<{
  reviewComments: ReviewCommentsState;
}>();

// Reuses ReviewFilesState.selectFile rather than a second editor.openRangeDiff call site (D10) —
// revealing the specific line inside the diff is not attempted (§10).
const emit = defineEmits<(e: 'select', path: string, origin: ReviewComment['origin']) => void>();

interface PathGroup {
  readonly path: string;
  readonly comments: ReviewComment[];
}

const groups = computed<readonly PathGroup[]>(() => {
  const out: PathGroup[] = [];
  for (const c of props.reviewComments.comments.value) {
    const last = out[out.length - 1];
    if (last && last.path === c.path) {
      last.comments.push(c);
    } else {
      out.push({ path: c.path, comments: [c] });
    }
  }
  return out;
});

const countLabel = computed(() => {
  const n = props.reviewComments.comments.value.length;
  return t('git.review.comments.count.label', { count: n });
});

function lineLabel(range: LineRange): string {
  return range.start === range.end
    ? t('git.review.comments.line.example', { start: range.start })
    : t('git.review.comments.lineRange.example', { start: range.start, end: range.end });
}

function kindLabel(c: ReviewComment): string {
  return c.kind === 'question'
    ? t('space.ade.needs.kind.question.label')
    : t('git.review.comments.kind.change.label');
}

function stateLabel(c: ReviewComment): string {
  return c.state === 'addressed'
    ? t('git.review.comments.state.addressed.label')
    : t('common.action.open.label');
}

/** D12's own two suffix forms, restated for the sidebar rather than the export's plain text. */
function anchorTitle(c: ReviewComment): string | undefined {
  const sha7 = shaLabel(c.anchorSha);
  switch (c.anchor) {
    case 'removed':
      return t('git.review.comments.anchorRemoved.hint', { sha: sha7 });
    case 'stale':
      return t('git.review.comments.anchorStale.hint', { sha: sha7 });
    case 'exact':
    case 'projected':
    case 'uncommitted':
      return undefined;
  }
}

// P131 Part 3 §5.3: one AttributeTooltip per list container, covering the anchor warning span's
// data-kira-tip — MountRoot.vue's own TooltipProvider (Part 2 §3.5) covers every real
// Tooltip/TooltipTrigger trio this file also uses.
const listEl = useTemplateRef<HTMLElement>('list');
</script>

<template>
  <div class="flex flex-col min-h-0 h-full">
    <ViewToolbar class="font-ui">
      <span class="text-muted-foreground text-graph-sm" data-testid="review-comments-count">{{
        countLabel
      }}</span>
      <TooltipIconButton
        icon="copy"
        :label="t('git.review.comments.copyForAi.tooltip')"
        class="ml-auto"
        :disabled="reviewComments.comments.value.length === 0"
        @click="reviewComments.copyForAi()"
      />
      <TooltipIconButton
        v-if="!reviewComments.confirmingClear.value"
        icon="clear-all"
        :label="t('git.review.comments.clearAll.tooltip')"
        :disabled="reviewComments.comments.value.length === 0 || reviewComments.pending.value"
        @click="reviewComments.confirmClear()"
      />
      <div v-else class="flex items-center gap-1 ml-auto text-graph-md">
        <Button variant="toolbar" size="kira" @click="reviewComments.clear()">
          {{
            t('git.review.comments.confirmClear.label', {
              count: reviewComments.comments.value.length,
            })
          }}
        </Button>
        <Button variant="toolbar" size="kira" @click="reviewComments.cancelClear()">{{
          t('common.action.cancel.label')
        }}</Button>
      </div>
    </ViewToolbar>

    <FieldError v-if="reviewComments.loadError.value" class="p-3">
      {{ t('git.review.comments.loadError.error', { error: reviewComments.loadError.value }) }}
    </FieldError>

    <template v-else>
      <template v-if="groups.length > 0">
        <section
          ref="list"
          class="flex-1 min-h-0 overflow-auto"
          :aria-label="t('git.review.pane.comments.label')"
        >
          <section v-for="group in groups" :key="group.path" :aria-label="group.path">
            <div class="pt-1 px-2 pb-0.5 font-data text-fg" aria-hidden="true">
              {{ group.path }}
            </div>
            <ul class="m-0 p-0 list-none">
              <li
                v-for="c in group.comments"
                :key="c.id"
                :class="rowVariants({ layout: 'double' })"
                class="flex-col items-stretch gap-0.5 pt-0.5 px-2 pb-1"
              >
                <div class="flex items-center gap-1">
                  <Button
                    variant="link"
                    size="inline"
                    class="font-data text-muted-foreground text-graph-sm after:absolute after:inset-0"
                    :aria-label="
                      t('git.review.comments.open.aria', {
                        path: group.path,
                        line: lineLabel(c.range),
                      })
                    "
                    @click="emit('select', group.path, c.origin)"
                  >
                    {{ lineLabel(c.range) }}
                  </Button>
                  <Badge
                    v-if="c.origin === 'working'"
                    variant="warn"
                    data-testid="review-comment-uncommitted"
                  >
                    {{ t('space.ade.changes.uncommitted.label') }}
                  </Badge>
                  <span
                    v-if="anchorTitle(c)"
                    class="codicon codicon-warning text-warn"
                    role="img"
                    :data-kira-tip="anchorTitle(c)"
                    :aria-label="anchorTitle(c)"
                  ></span>
                  <span
                    class="text-graph-sm text-muted-foreground"
                    data-testid="review-comment-chips"
                    >{{
                      t('common.dotPair.label', { first: kindLabel(c), second: stateLabel(c) })
                    }}</span
                  >
                  <TooltipIconButton
                    icon="trash"
                    :label="t('git.review.comments.delete.tooltip')"
                    class="relative z-2 ml-auto"
                    :disabled="reviewComments.pending.value"
                    @click="reviewComments.remove(c.id)"
                  />
                </div>
                <p class="m-0 pl-2.5 whitespace-pre-wrap text-fg text-graph-md">{{ c.body }}</p>
                <p
                  v-if="c.reply"
                  class="m-0 pl-2.5 whitespace-pre-wrap text-muted-foreground text-graph-md"
                >
                  {{ c.reply }}
                </p>
              </li>
            </ul>
          </section>
        </section>
        <AttributeTooltip :container="listEl" />
      </template>

      <p v-else-if="!reviewComments.loading.value" class="m-0 p-3 text-muted-foreground">
        {{ t('git.review.comments.empty.emptyHint') }}
      </p>

      <InlineState v-if="reviewComments.loading.value && groups.length === 0" kind="loading">
        {{ t('common.state.loading.loading') }}
      </InlineState>
    </template>
  </div>
</template>
