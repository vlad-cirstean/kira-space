<script setup lang="ts">
import type { TransformView } from '@shared/domain/dbmcp';
import type { KeptUse } from '@shared/domain/kept';
import HighlightedText from '@theme/components/HighlightedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { KEPT_MARK_CLASS, keptScriptRanges } from '../kept/keptHighlight';
import JsScriptBlock from './JsScriptBlock.vue';

// A transform script in an approval card: the source the user approves, with its kept calls marked, the
// names it may keep and a note when it reads kept values. The script only shapes what the agent gets back.
// `testid` prefixes the may-keep and reads rows; `scriptTestid` is the script block's own.
const props = defineProps<{
  label: string;
  script: TransformView;
  kept: readonly KeptUse[];
  testid: string;
  scriptTestid: string;
}>();

const sites = computed(() => [
  ...props.kept.flatMap((u) => u.sites),
  ...props.script.mayKeep.flatMap((m) => m.sites),
]);
const marked = computed(() => props.kept.length > 0 || props.script.mayKeep.length > 0);
// Only a read in the transform counts: a read in prepare is marked in the prepare block.
const readsInScript = computed(() =>
  props.kept.some((u) => u.sites.some((s) => s.field === 'script' && s.part === 'transform')),
);
</script>

<template>
  <div class="flex flex-col gap-1">
    <JsScriptBlock
      :label="label"
      :source="script.source"
      :range-highlights="marked ? keptScriptRanges(sites, 'transform') : undefined"
      :testid="scriptTestid"
    />
    <p v-if="script.mayKeep.length" class="m-0 text-kira-sm" :data-testid="`${testid}-may-keep`">
      {{ t('studio.kept.mayKeep.label') }}
      <span v-for="m in script.mayKeep" :key="m.name" class="mr-2 inline-flex items-center gap-1">
        <HighlightedText
          class="font-data"
          :parts="[{ text: m.name, match: true }]"
          :mark-class="KEPT_MARK_CLASS"
        />
        <Badge
          :variant="m.secret ? 'warn' : 'info'"
          :data-testid="`${testid}-may-keep-${m.name}-${m.secret ? 'secret' : 'plain'}`"
          >{{
            t(m.secret ? 'common.secret.label' : 'studio.kept.kind.plain.label')
          }}</Badge
        >
      </span>
    </p>
    <FieldDescription v-if="readsInScript" :data-testid="`${testid}-script-reads`">{{
      t('studio.kept.scriptReads.hint')
    }}</FieldDescription>
  </div>
</template>
