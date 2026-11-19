<script setup lang="ts">
import type { DbQueryPayload } from '@shared/domain/dbmcp';
import type { KeptUse } from '@shared/domain/kept';
import HighlightedText from '@theme/components/HighlightedText.vue';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { KEPT_MARK_CLASS, keptScriptRanges } from '../kept/keptHighlight';
import JsScriptBlock from './JsScriptBlock.vue';

// The result script of a run_query card: the source the user approves, with its kept calls marked, the
// names it may keep and a note when it reads kept values. The statement above is what runs; the script only
// shapes what the agent gets back.
const props = defineProps<{
  script: NonNullable<DbQueryPayload['script']>;
  kept: readonly KeptUse[];
}>();

const sites = computed(() => [
  ...props.kept.flatMap((u) => u.sites),
  ...props.script.mayKeep.flatMap((m) => m.sites),
]);
const marked = computed(() => props.kept.length > 0 || props.script.mayKeep.length > 0);
const readsInScript = computed(() =>
  props.kept.some((u) => u.sites.some((s) => s.field === 'script')),
);
</script>

<template>
  <div class="flex flex-col gap-1">
    <JsScriptBlock
      :label="t('studio.dbMcp.approval.query.scriptResult.label')"
      :source="script.source"
      :range-highlights="marked ? keptScriptRanges(sites, 'transform') : undefined"
      testid="db-approval-script"
    />
    <p v-if="script.mayKeep.length" class="m-0 text-kira-sm" data-testid="db-approval-may-keep">
      {{ t('studio.kept.mayKeep.label') }}
      <HighlightedText
        v-for="m in script.mayKeep"
        :key="m.name"
        class="mr-1 font-data"
        :parts="[{ text: m.name, match: true }]"
        :mark-class="KEPT_MARK_CLASS"
      />
    </p>
    <FieldDescription v-if="readsInScript" data-testid="db-approval-script-reads">{{
      t('studio.kept.scriptReads.hint')
    }}</FieldDescription>
  </div>
</template>
