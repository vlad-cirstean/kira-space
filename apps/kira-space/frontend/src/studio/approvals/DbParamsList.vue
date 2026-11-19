<script setup lang="ts">
import type { KeptUse } from '@shared/domain/kept';
import KeyValueList from '@theme/components/KeyValueList.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import KeptNameMark from '../kept/KeptNameMark.vue';
import { keptTypeLabel } from '../kept/keptFormat';

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
    <SectionHeading flush :label="t('common.field.parameters.label')" />
    <KeyValueList density="compact">
      <template v-for="(row, i) in rows" :key="i">
        <dt class="font-data" :data-testid="`db-approval-param-${i}-placeholder`">
          {{ row.placeholder }}
        </dt>
        <dd :data-testid="`db-approval-param-${i}-value`">
          <KeptNameMark :name="row.ref" :type="row.type" />
        </dd>
      </template>
    </KeyValueList>
  </section>
</template>
