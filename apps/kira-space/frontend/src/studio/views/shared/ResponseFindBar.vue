<script setup lang="ts">
import FindBar from '@workbench/components/FindBar.vue';
import SearchOptionToggles from '@workbench/components/SearchOptionToggles.vue';
import { computed, ref, watch } from 'vue';
import { type FindOptions, findQueryIsInvalid, findRanges } from '../../editor/findRanges';

// Find bar over the response body and the raw exchange's documents: matches are numbered across
// all targets in order. The host pane paints them from the exposed query, cursor and options.

export interface FindBarHost {
  scrollRangeIntoView(from: number, to: number): void;
}

export interface FindBarTarget {
  doc: string;
  host: FindBarHost | null;
}

// One target for the Body pane, two for the Raw pane (request wire, response wire).
const props = defineProps<{ targets: readonly FindBarTarget[] }>();
const emit = defineEmits<{ close: [] }>();

const query = ref('');
// 0-based, across every target's matches concatenated in order.
const currentGlobal = ref(0);

const matchCase = ref(false);
const wholeWord = ref(false);
const regex = ref(false);
const options = computed<FindOptions>(() => ({
  matchCase: matchCase.value,
  wholeWord: wholeWord.value,
  regex: regex.value,
}));
// A half-typed regex marks the input instead of clearing the highlights.
const invalid = computed(() => findQueryIsInvalid(query.value, options.value));

const matchCounts = computed(() =>
  props.targets.map((t) => findRanges(t.doc, query.value, undefined, options.value).length),
);
const totalMatches = computed(() => matchCounts.value.reduce((a, b) => a + b, 0));
const displayIndex = computed(() =>
  totalMatches.value === 0 ? 0 : Math.min(currentGlobal.value, totalMatches.value - 1) + 1,
);

function scrollToCurrent(): void {
  if (totalMatches.value === 0) return;
  let remaining = Math.min(currentGlobal.value, totalMatches.value - 1);
  for (let i = 0; i < props.targets.length; i++) {
    const count = matchCounts.value[i] ?? 0;
    if (remaining < count) {
      const ranges = findRanges(props.targets[i]?.doc ?? '', query.value, undefined, options.value);
      const r = ranges[remaining];
      if (r) props.targets[i]?.host?.scrollRangeIntoView(r.from, r.to);
      return;
    }
    remaining -= count;
  }
}

// An option change alters the match set, so the cursor resets like a new query does.
watch([query, options], () => {
  currentGlobal.value = 0;
});
watch(options, scrollToCurrent);

function goNext(): void {
  if (totalMatches.value === 0) return;
  currentGlobal.value = (currentGlobal.value + 1) % totalMatches.value;
  scrollToCurrent();
}
function goPrev(): void {
  if (totalMatches.value === 0) return;
  currentGlobal.value = (currentGlobal.value - 1 + totalMatches.value) % totalMatches.value;
  scrollToCurrent();
}

function close(): void {
  emit('close');
}

defineExpose({ query, currentGlobal, options });
</script>

<template>
  <FindBar
    v-model="query"
    :position="displayIndex"
    :total="totalMatches"
    :invalid="invalid"
    testid-prefix="http-find-"
    @query="scrollToCurrent"
    @next="goNext"
    @prev="goPrev"
    @close="close"
  >
    <template #options>
      <SearchOptionToggles
        v-model:match-case="matchCase"
        v-model:whole-word="wholeWord"
        v-model:regex="regex"
        testid-prefix="http-find-"
      />
    </template>
  </FindBar>
</template>
