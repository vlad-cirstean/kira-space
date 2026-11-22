<script setup lang="ts">
import HighlightedText from '@theme/components/HighlightedText.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import { Button } from '@theme/components/ui/button';
import { Collapsible, CollapsibleTrigger } from '@theme/components/ui/collapsible';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';

// A name/value list in the data font, values exactly as sent. Over three rows the rest hides behind "+N more".
const props = withDefaults(
  defineProps<{
    rows: { name: string; value: string }[];
    testid: string;
    rowTestid?: (name: string) => string;
    /** Marks parts of a row's name or value: the ranges to mark, or none. */
    highlight?: (
      row: number,
      part: 'name' | 'value',
    ) => readonly { text: string; match: boolean }[] | null;
    markClass?: string;
    density?: 'default' | 'approval';
    /** The prepared key of a row the script set, or none: marks the row (`data-prepared`) with `preparedClass`. */
    preparedOf?: (row: number) => string | undefined;
    preparedClass?: string;
  }>(),
  { density: 'default' },
);

const VISIBLE = 3;
const open = ref(false);
const shown = computed(() => (open.value ? props.rows : props.rows.slice(0, VISIBLE)));
const hidden = computed(() => Math.max(0, props.rows.length - VISIBLE));
</script>

<template>
  <Collapsible v-model:open="open" :data-testid="testid">
    <KeyValueList :density="density">
      <div
        v-for="(row, i) in shown"
        :key="`${i}-${row.name}`"
        class="contents"
        :data-testid="rowTestid?.(row.name)"
      >
        <dt class="font-data">
          <HighlightedText
            v-if="highlight?.(i, 'name')"
            :parts="highlight(i, 'name') ?? []"
            :mark-class="markClass"
          />
          <template v-else>{{ row.name }}</template>
        </dt>
        <dd
          class="font-data"
          :class="preparedOf?.(i) && preparedClass"
          :data-prepared="preparedOf?.(i)"
        >
          <HighlightedText
            v-if="highlight?.(i, 'value')"
            :parts="highlight(i, 'value') ?? []"
            :mark-class="markClass"
          />
          <template v-else>{{ row.value }}</template>
        </dd>
      </div>
    </KeyValueList>
    <CollapsibleTrigger v-if="hidden > 0 && !open" as-child>
      <Button variant="link-meta" size="inline" data-testid="approval-more">
        {{ t('common.more.label', { count: hidden }) }}
      </Button>
    </CollapsibleTrigger>
  </Collapsible>
</template>
