<script setup lang="ts">
import { type ConnectionKind, connectionKindSchema } from '@shared/domain/connection';
import SearchField from '@theme/components/SearchField.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { RadioGroup, RadioGroupCard } from '@theme/components/ui/radio-group';
import { t } from '@theme/i18n';
import { matchesQuery } from '@workbench/behaviour/match';
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
  <div class="flex flex-col gap-3 min-h-0 flex-1">
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
      <TextTooltip
        v-for="option in filteredKinds"
        :key="option"
        :text="
          SUPPORTED_KINDS.has(option)
            ? ''
            : t('studio.project.connectionDialog.kind.unsupported.tooltip', {
                kind: KIND_LABEL[option],
              })
        "
      >
        <RadioGroupCard
          :value="option"
          :disabled="!SUPPORTED_KINDS.has(option)"
          :data-testid="`connection-kind-${option}`"
          @click="pick(option)"
        >
          <template #title>
            <span class="flex items-center gap-1.5">
              <span
                class="flex"
                :class="
                  SUPPORTED_KINDS.has(option) ? KIND_ACCENT_CLASS[option] : 'text-muted-foreground'
                "
              >
                <EngineIcon :kind="option" :size="16" />
              </span>
              {{ KIND_LABEL[option] }}
            </span>
          </template>
        </RadioGroupCard>
      </TextTooltip>
    </RadioGroup>
  </div>
</template>
