<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { computed, ref, watch } from 'vue';

// The first/prev/page-jump/next/last pager DataToolbar.vue and DocumentView.vue each
// wrote out — same five controls, same order, same pageDisplay/pageInputValue/pageCount/onJump
// computeds, differing only in their data-testid prefix and the "count first" tooltip's noun.
// Uses the document view's own icon spelling (arrow-left/arrow-right for prev/next) — the
// grid's own chevron-left/chevron-right pair for First and Previous rendered two identical
// glyphs side by side, which this fixes by construction.
const props = defineProps<{
  pageIndex: number;
  pageSize: number;
  count: number | null;
  hasMore: boolean;
  testidPrefix: string;
  lastTooltip: string;
  strategy?: string;
}>();

const emit = defineEmits<{ first: []; prev: []; next: []; last: []; jump: [pageIndex: number] }>();

const pageDisplay = computed(() => props.pageIndex + 1);

// A plain `:value="pageDisplay"` fights the user's typing: any unrelated reactive read this
// component makes forces a re-render, and Vue reasserts the bound value on the DOM input
// regardless of whether pageDisplay itself changed — wiping out whatever the user has typed but
// not yet committed. Mirroring it through its own ref, kept in sync with pageDisplay only when
// the page advances, avoids the fight.
const pageInputValue = ref(String(pageDisplay.value));
watch(pageDisplay, (v) => {
  pageInputValue.value = String(v);
});

const pageCount = computed(() => {
  if (props.count === null || !props.pageSize) return null;
  return Math.max(1, Math.ceil(props.count / props.pageSize));
});

// A rejected jump (non-numeric, or < 1) used to leave
// pageInputValue exactly as the user left it forever — the box's own resync only ever runs off
// `watch(pageDisplay, ...)` above, which fires only when the page changes, so a rejected
// jump (nothing changes) never touched it. And a jump past the last known page was never clamped
// here at all (state.ts's own goToPage only clamps at 0), so typing e.g. 99 against a 3-page
// result silently asked for an offset far past the end.
function onJump(e: Event): void {
  const value = Number((e.target as HTMLInputElement).value);
  const valid = Number.isInteger(value) && value >= 1;
  if (!valid) {
    // Resync to the actual current page rather than leaving the box permanently out of sync with
    // what the grid is really showing.
    pageInputValue.value = String(pageDisplay.value);
    return;
  }
  const target = pageCount.value !== null ? Math.min(value, pageCount.value) : value;
  emit('jump', target - 1);
  // Reflect a clamp immediately rather than waiting on the parent to round-trip pageIndex back
  // through the pageDisplay watcher — an unclamped jump doesn't need this, since that watcher
  // already fires once the new pageIndex prop lands.
  if (target !== value) pageInputValue.value = String(target);
}
</script>

<template>
  <div class="group gap-0.5" :data-testid="`${testidPrefix}pager`" :data-pagination="strategy">
    <TooltipIconButton
      icon="chevron-left"
      :label="t('studio.pager.firstPage.label')"
      disabled-trigger
      :data-testid="`${testidPrefix}pager-first`"
      :disabled="pageIndex === 0"
      @click="emit('first')"
    />
    <TooltipIconButton
      icon="arrow-left"
      :label="t('common.action.previousPage.label')"
      disabled-trigger
      :data-testid="`${testidPrefix}pager-prev`"
      :disabled="pageIndex === 0"
      @click="emit('prev')"
    />
    <span
      class="inline-flex items-center whitespace-nowrap gap-0.5 text-kira-sm text-muted-foreground"
    >
      <CatalogText
        k="common.ofTotal.label"
        :params="{ total: pageCount ?? '?' }"
        class="inline-flex items-center gap-0.5"
      >
        <template #value>
          <div class="w-14">
            <Input
              v-model="pageInputValue"
              type="number"
              min="1"
              step="1"
              size="kira"
              class="w-full text-center"
              :data-testid="`${testidPrefix}pager-page-input`"
              @change="onJump"
            />
          </div>
        </template>
      </CatalogText>
    </span>
    <TooltipIconButton
      icon="arrow-right"
      :label="t('common.action.nextPage.label')"
      disabled-trigger
      :data-testid="`${testidPrefix}pager-next`"
      :disabled="!hasMore"
      @click="emit('next')"
    />
    <TooltipIconButton
      icon="chevron-right"
      :label="pageCount ? t('studio.pager.lastPage.label') : lastTooltip"
      :aria-label="t('studio.pager.lastPage.label')"
      disabled-trigger
      :data-testid="`${testidPrefix}pager-last`"
      :disabled="!pageCount"
      @click="emit('last')"
    />
  </div>
</template>
