<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import PickerPopover from '@workbench/components/pickers/PickerPopover.vue';
import type { PickerGroup, PickerOption } from '@workbench/lib/pickerOptions';
import { computed, ref } from 'vue';
import { CATEGORY_ORDER, categoryLabel, kindIcon, POD_KIND } from '../kinds';
import type { KubeKind } from '../wire';

const props = defineProps<{
  kinds: readonly KubeKind[];
  /** Key of the kind shown. */
  value: string;
  /** Keys of the kinds picked lately, oldest first. */
  recent: readonly string[];
  /** Some API groups did not answer. */
  partial: boolean;
}>();
const emit = defineEmits<{ select: [key: string] }>();

const open = ref(false);

const keyId = (key: string): string => key.replaceAll('/', '_');

const chips = computed(() => {
  const byKey = new Map(props.kinds.map((k) => [k.key, k]));
  return [POD_KIND, ...props.recent].flatMap((key) => {
    const k = byKey.get(key);
    return k
      ? [
          {
            value: k.key,
            label: k.kind,
            icon: kindIcon(k),
            testid: `kube-kind-chip-${keyId(k.key)}`,
          },
        ]
      : [];
  });
});

const groups = computed<PickerGroup[]>(() =>
  CATEGORY_ORDER.map((c) => ({ id: c, label: categoryLabel(c) })),
);
const options = computed<PickerOption[]>(() =>
  props.kinds.map((k) => ({
    value: k.key,
    label: k.kind,
    detail: k.group ? `${k.group}/${k.version}` : k.version,
    group: k.category,
    keywords: [k.resource],
    testid: keyId(k.key),
  })),
);
</script>

<template>
  <div class="flex min-w-0 items-center gap-1" data-testid="kube-kind-bar">
    <SecondaryTabs
      :model-value="value"
      :items="chips"
      @update:model-value="(v) => emit('select', v)"
    />
    <PickerPopover
      v-model:open="open"
      :model-value="value"
      :options="options"
      :groups="groups"
      :placeholder="t('kubernetes.kinds.more.label')"
      :trigger-label="t('kubernetes.kinds.more.label')"
      :search-placeholder="t('kubernetes.kinds.picker.search.placeholder')"
      :empty-text="t('common.state.noMatches.empty')"
      size="kira"
      trigger="toolbar"
      icon="list-flat"
      testid="kube-kind-more"
      @select="(o) => emit('select', o.value)"
    />
    <Tooltip v-if="partial">
      <TooltipTrigger as-child>
        <CodiconIcon
          name="warning"
          :size="13"
          class="shrink-0 text-muted-foreground"
          data-testid="kube-kinds-partial"
        />
      </TooltipTrigger>
      <TooltipContent>{{ t('kubernetes.kinds.partial.hint') }}</TooltipContent>
    </Tooltip>
  </div>
</template>
