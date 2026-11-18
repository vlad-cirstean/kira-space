<script setup lang="ts">
import type { DistillRewrite } from '@shared/domain/memoryDistill';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import type { MessageKey } from '@theme/i18n';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { storeToRefs } from 'pinia';
import { computed } from 'vue';
import { copyAndAnnounce } from '../../behaviour/clipboard';
import { formatCurrency, formatPercent } from '../../behaviour/format';
import { useDistillUiStore } from './distillStore';

// P315: a rewrite-mode result: the text in its own form, why it was kept when a gate rejected the
// draft, and what the rewrite left out. Copy only; the output is text, not memory lines.
const ui = useDistillUiStore();
const { result } = storeToRefs(ui);

const KEPT = {
  grew: 'memory.distill.rewrite.kept.grew.body',
  form: 'memory.distill.rewrite.kept.form.body',
  indent: 'memory.distill.rewrite.kept.indent.body',
  width: 'memory.distill.rewrite.kept.width.body',
  directive: 'memory.distill.rewrite.kept.directive.body',
  tokens: 'memory.distill.rewrite.kept.tokens.body',
  output: 'memory.distill.rewrite.kept.output.body',
  hedge: 'memory.distill.rewrite.kept.hedge.body',
} as const satisfies Record<Exclude<DistillRewrite['gate'], ''>, MessageKey>;

const WHY = {
  tag: 'memory.distill.rewrite.why.tag.label',
  history: 'common.module.history.label',
  restatesCode: 'memory.distill.rewrite.why.restatesCode.label',
  trivia: 'memory.distill.rewrite.why.trivia.label',
} as const satisfies Record<DistillRewrite['dropped'][number]['why'], MessageKey>;

const rewrite = computed(() => result.value?.rewrite ?? null);
const keptKey = computed(() => (rewrite.value?.gate ? KEPT[rewrite.value.gate] : null));
const unchanged = computed(() => !!rewrite.value && !rewrite.value.changed && !rewrite.value.gate);
const summary = computed(() => {
  const r = result.value;
  if (!r) return '';
  return t('memory.distill.result.summary.status', {
    inChars: r.inChars,
    outChars: r.outChars,
    percent: formatPercent(r.inChars > 0 ? (r.outChars / r.inChars) * 100 : 0),
    inTokens: r.inTokens,
    outTokens: r.outTokens,
    cost: formatCurrency(r.costUsd),
  });
});

function copy(): void {
  if (!rewrite.value) return;
  void copyAndAnnounce(rewrite.value.text, t('memory.distill.rewrite.copy.announce'));
}
</script>

<template>
  <div v-if="rewrite" class="flex flex-col gap-2" data-testid="distill-result">
    <FieldDescription data-testid="distill-summary">{{ summary }}</FieldDescription>
    <pre
      class="m-0 overflow-x-auto font-data text-kira-md whitespace-pre"
      data-testid="distill-rewrite-text"
    >{{ rewrite.text }}</pre>
    <FieldDescription v-if="unchanged" data-testid="distill-rewrite-unchanged">{{
      t('memory.distill.rewrite.unchanged.body')
    }}</FieldDescription>
    <FieldDescription v-else-if="keptKey" data-testid="distill-rewrite-kept">{{
      t(keptKey)
    }}</FieldDescription>
    <section
      v-if="rewrite.changed && rewrite.dropped.length > 0"
      class="flex flex-col gap-1"
      data-testid="distill-rewrite-dropped"
    >
      <SectionHeading flush :label="t('memory.distill.rewrite.dropped.title')" />
      <ul class="m-0 flex list-none flex-col gap-1 p-0">
        <li
          v-for="(item, i) in rewrite.dropped"
          :key="i"
          class="flex flex-col"
          :data-testid="`distill-rewrite-dropped-${i}`"
        >
          <span class="text-kira-md">{{ item.text }}</span>
          <span class="text-kira-sm text-muted-foreground">{{ t(WHY[item.why]) }}</span>
        </li>
      </ul>
    </section>
    <div class="flex flex-wrap items-center gap-1.5">
      <Button variant="toolbar" size="kira" data-testid="distill-rewrite-copy" @click="copy"
        ><CodiconIcon :name="ICON.copy" :size="13" />{{ t('common.action.copy.label') }}</Button
      >
    </div>
  </div>
</template>
