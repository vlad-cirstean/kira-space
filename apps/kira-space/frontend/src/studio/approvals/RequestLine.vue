<script setup lang="ts">
import { httpMethodToken } from '@shared/domain/http';
import HighlightedText from '@theme/components/HighlightedText.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { methodTextClass } from '@theme/methodColor';
import { computed } from 'vue';
import { KEPT_MARK_CLASS, type KeptRange, keptParts } from '../kept/keptHighlight';
import { tokenizeRequestUrl, type UrlToken, type UrlTokenKind } from './requestLine';

// The request line, each part in its own colour. Origin and path on line 1, then one
// query pair per line; a value cut at the line end shows its full text in a tooltip.
const props = defineProps<{
  method: string;
  url: string;
  kept: readonly KeptRange[];
}>();

const methodToken = computed(() => httpMethodToken(props.method));
const tokens = computed(() => tokenizeRequestUrl(props.url));

const TOKEN_CLASS: Record<UrlTokenKind, string> = {
  scheme: 'font-semibold text-fg',
  host: 'font-semibold text-fg',
  port: 'font-semibold text-fg',
  path: 'text-fg',
  var: 'text-var-resolved',
  sep: 'text-syntax-meta',
  key: 'text-syntax-property',
  value: 'text-syntax-string',
  hash: 'text-muted-foreground',
};

function classOf(tk: UrlToken, inOrigin: boolean): string[] {
  return tk.kind === 'var' && inOrigin
    ? [TOKEN_CLASS[tk.kind], 'font-semibold']
    : [TOKEN_CLASS[tk.kind]];
}

// Kept-value marks inside one token, in token-local offsets.
function partsOf(tk: UrlToken) {
  const local = props.kept
    .filter((r) => r.to > tk.from && r.from < tk.from + tk.text.length)
    .map((r) => ({
      from: Math.max(0, r.from - tk.from),
      to: Math.min(tk.text.length, r.to - tk.from),
    }));
  return local.length ? keptParts(tk.text, local) : null;
}
</script>

<template>
  <div
    class="flex flex-wrap items-baseline font-data text-kira-md leading-normal"
    data-testid="api-approval-url"
  >
    <span
      class="mr-2 font-semibold"
      :class="methodTextClass(methodToken)"
      data-token="method"
      :data-method="methodToken"
      >{{
        method
      }}</span
    ><span class="max-w-full break-all" data-testid="api-approval-url-origin"
      ><span
        v-for="(tk, i) in tokens.origin"
        :key="i"
        :class="classOf(tk, true)"
        :data-token="tk.kind"
        ><HighlightedText v-if="partsOf(tk)" :parts="partsOf(tk)!" :mark-class="KEPT_MARK_CLASS" />
        <template v-else>{{ tk.text }}</template></span
      ></span
    ><span
      v-for="(seg, s) in tokens.segments"
      :key="`s${s}`"
      class="max-w-full break-all"
      data-testid="api-approval-url-segment"
      ><span v-for="(tk, i) in seg" :key="i" :class="classOf(tk, false)" :data-token="tk.kind"
        ><HighlightedText v-if="partsOf(tk)" :parts="partsOf(tk)!" :mark-class="KEPT_MARK_CLASS" />
        <template v-else>{{ tk.text }}</template></span
      ></span
    ><span
      v-for="(pair, p) in tokens.pairs"
      :key="`p${p}`"
      class="flex min-w-0 max-w-full basis-full pl-4"
      data-testid="api-approval-query-pair"
      ><span class="shrink-0 break-all"
        ><span
          v-for="(tk, i) in pair.lead"
          :key="i"
          :class="classOf(tk, false)"
          :data-token="tk.kind"
          ><HighlightedText
            v-if="partsOf(tk)"
            :parts="partsOf(tk)!"
            :mark-class="KEPT_MARK_CLASS"
          /><template v-else>{{ tk.text }}</template></span
        ></span
      ><span
        v-if="pair.value.length"
        class="flex min-w-0 flex-1"
        data-testid="api-approval-query-value"
        ><TruncatedText :text="pair.valueText"
          ><span
            v-for="(tk, i) in pair.value"
            :key="i"
            :class="classOf(tk, false)"
            :data-token="tk.kind"
            ><HighlightedText
              v-if="partsOf(tk)"
              :parts="partsOf(tk)!"
              :mark-class="KEPT_MARK_CLASS"
            /><template v-else>{{ tk.text }}</template></span
          ></TruncatedText
        ></span
      ></span
    ><span
      v-for="(tk, i) in tokens.hash"
      :key="`h${i}`"
      class="break-all"
      :class="classOf(tk, false)"
      data-token="hash"
      >{{
        tk.text
      }}</span
    >
  </div>
</template>
