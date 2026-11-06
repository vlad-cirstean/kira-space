<script setup lang="ts">
import TreeTwisty from '@workbench/components/TreeTwisty.vue';
import { wheelToHorizontal } from '@workbench/lib/wheelScroll';
import { computed, ref } from 'vue';
import { useDocumentRowsStore } from './rows';

// One expanded document's body, rendered as flat indented key/value lines out of visibleLines()
// (P27 D19) — no CodeMirror, no per-node component recursion, so the DOM cost here is linear in
// what is actually on screen rather than in the document's total node count.
const props = defineProps<{ tabId: string; row: number }>();
const emit = defineEmits<{ togglePath: [path: string] }>();

const documentRowsStore = useDocumentRowsStore();

const lines = computed(() => {
  void documentRowsStore.rowsVersion.n;
  return documentRowsStore.visibleLines(props.tabId, props.row);
});

// A long scalar value is revealed by scrolling this list sideways rather than
// by wrapping it — rows.ts's rowHeight() is exact and measurement-free (one LINE_H per visible
// node), so a wrapped line would break the VirtualList prefix sum that depends on it. Same
// TabStrip.vue/ConsoleView.vue idiom: chrome-less scrolling (the CSS below) plus a wheel handler,
// since a plain mouse wheel produces only a vertical axis.
const treeRef = ref<HTMLElement | null>(null);
function onWheel(e: WheelEvent): void {
  if (wheelToHorizontal(treeRef.value, e)) e.preventDefault();
}

// D12: coloured with the existing --kira-syntax-* tokens — 'bson' (a shell constructor call like
// ObjectId(...), or a canonical-EJSON fallback for a type this app's shell has no constructor for)
// reuses --kira-syntax-function, the same hue CodeMirror already gives a function-call name.
const TOKEN_CLASS: Record<'string' | 'number' | 'keyword' | 'bson', string> = {
  string: 'text-syntax-string',
  number: 'text-syntax-number',
  keyword: 'text-syntax-keyword',
  bson: 'text-syntax-function',
};
</script>

<template>
  <!-- P43 iter3 D42: chrome-less horizontal scrolling, TabStrip.vue's/ConsoleView.vue's own idiom —
       the same three declarations, occupying zero vertical space, so rowHeight()'s exact LINE_H
       accounting (rows.ts) is untouched. -->
  <div
    class="overflow-x-auto overflow-y-hidden text-kira-sm font-data scrollbar-none py-1"
    data-testid="document-tree"
    ref="treeRef"
    @wheel="onWheel"
  >
    <!-- A line only as wide as its own content grows the scroller's scrollWidth past the panel — a
         plain 100% width would clip at the viewport instead of revealing the rest on scroll. -->
    <div
      v-for="line in lines"
      :key="line.node.path"
      class="flex items-center whitespace-nowrap w-max min-w-full gap-0.5 pr-2 h-4.5"
      :style="{ paddingLeft: `${line.depth * 16 + 4}px` }"
      data-testid="document-tree-line"
      :data-path="line.node.path"
      :data-depth="line.depth"
    >
      <TreeTwisty
        v-if="line.expandable"
        :expanded="line.expanded"
        has-children
        testid="document-tree-twisty"
        @toggle="emit('togglePath', line.node.path)"
      />
      <span v-else class="shrink-0 w-3.5"></span>
      <span v-if="line.node.key !== ''" class="shrink-0 text-syntax-property"
        >{{ line.node.key }}:</span
      >
      <!-- P43 iter3 D42: no longer clipped — the row's own max-content width lets this grow past
           the panel instead, reachable by scrolling the tree sideways. -->
      <span
        v-if="line.node.kind === 'scalar'"
        class="shrink-0"
        :class="TOKEN_CLASS[line.node.token]"
        data-testid="document-tree-value"
        >{{
          line.node.text
        }}</span
      >
      <span v-else class="shrink-0 text-muted-foreground" data-testid="document-tree-summary">{{
        line.node.summary
      }}</span>
    </div>
  </div>
</template>
