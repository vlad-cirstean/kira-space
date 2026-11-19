<script setup lang="ts">
import type { KeptUse } from '@shared/domain/kept';
import KeyValueList from '@theme/components/KeyValueList.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { keptTypeLabel } from '../kept/keptFormat';
import { KEPT_MARK_CLASS } from '../kept/keptHighlight';

// Which kept variable fills each placeholder of a statement: `$1`, `? 1` or `{ids:Array(UInt64)}` on
// the left, the variable inside the same mark as in the statement on the right. Names and types only.
const props = defineProps<{
  params: readonly string[];
  placeholders: readonly string[];
  kept: readonly KeptUse[];
}>();

const rows = computed(() =>
  props.params.map((name, i) => ({
    placeholder: props.placeholders[i] ?? '',
    ref: `{{kept.${name}}}`,
    type: keptTypeLabel(
      props.kept.find((u) => u.name === name) ?? { type: 'null', items: 0, bytes: 0 },
    ),
  })),
);
</script>

<template>
  <section class="flex flex-col gap-1" data-testid="db-approval-params">
    <SectionHeading flush :label="t('automations.params.title.label')" />
    <KeyValueList density="compact">
      <template v-for="(row, i) in rows" :key="i">
        <dt class="font-data" :data-testid="`db-approval-param-${i}-placeholder`">
          {{ row.placeholder }}
        </dt>
        <dd class="font-data" :data-testid="`db-approval-param-${i}-value`">
          <mark :class="['rounded-kira-sm text-inherit', KEPT_MARK_CLASS]">{{ row.ref }}</mark>
          <span class="text-muted-foreground"> · {{ row.type }}</span>
        </dd>
      </template>
    </KeyValueList>
  </section>
</template>
