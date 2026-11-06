<script setup lang="ts">
import { type ConnectionKind, connectionKindSchema } from '@shared/domain/connection';
import SearchField from '@theme/components/SearchField.vue';
import { RadioGroup } from '@theme/components/ui/radio-group';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { matchesQuery } from '@workbench/behaviour/match';
import { RadioGroupItem } from 'reka-ui';
import { computed, ref } from 'vue';
import EngineIcon from '../../theme/EngineIcon.vue';
import { KIND_ACCENT_CLASS, KIND_LABEL, SUPPORTED_KINDS } from './kinds';

defineProps<{ kind: ConnectionKind }>();
const emit = defineEmits<{ pick: [kind: ConnectionKind] }>();

const search = ref('');
const filteredKinds = computed(() =>
  connectionKindSchema.options.filter((kind) => matchesQuery(KIND_LABEL[kind], search.value)),
);

// Bound to both change and click: change alone never fires when re-clicking the checked kind.
function pick(kind: ConnectionKind): void {
  if (SUPPORTED_KINDS.has(kind)) emit('pick', kind);
}
</script>

<template>
  <div class="flex flex-col gap-3 p-3 min-h-0 flex-1">
    <SearchField
      v-model="search"
      size="kira-lg"
      :placeholder="t('studio.project.connectionDialog.engineSearch.placeholder')"
      class="font-ui"
      data-testid="connection-engine-search"
    />

    <RadioGroup
      class="grid-cols-3 gap-1.5 content-start"
      :aria-label="t('studio.project.connectionDialog.kind.aria')"
      :model-value="kind"
      data-testid="connection-kind"
      @update:model-value="(v) => pick(String(v) as ConnectionKind)"
    >
      <Tooltip v-for="option in filteredKinds" :key="option">
        <TooltipTrigger as-child>
          <!-- reka's own item: the theme RadioGroupItem is the 14px radio dot, this is a card. -->
          <RadioGroupItem
            :value="option"
            :disabled="!SUPPORTED_KINDS.has(option)"
            :aria-label="KIND_LABEL[option]"
            :data-testid="`connection-kind-${option}`"
            class="relative flex flex-col items-start gap-1 rounded-kira border bg-bg px-2 py-3 text-left text-inherit not-data-[disabled]:hover:border-border not-data-[disabled]:hover:bg-field data-[disabled]:opacity-50"
            :class="kind === option ? 'border-focus' : 'border-border'"
            @click="pick(option)"
          >
            <span
              class="flex mb-1"
              :class="
                SUPPORTED_KINDS.has(option) ? KIND_ACCENT_CLASS[option] : 'text-muted-foreground'
              "
            >
              <EngineIcon :kind="option" :size="22" />
            </span>
            <span class="text-kira-md text-fg">{{ KIND_LABEL[option] }}</span>
          </RadioGroupItem>
        </TooltipTrigger>
        <TooltipContent>{{
          SUPPORTED_KINDS.has(option)
            ? KIND_LABEL[option]
            : t('studio.project.connectionDialog.kind.unsupported.tooltip', {
                kind: KIND_LABEL[option],
              })
        }}</TooltipContent>
      </Tooltip>
    </RadioGroup>
  </div>
</template>
