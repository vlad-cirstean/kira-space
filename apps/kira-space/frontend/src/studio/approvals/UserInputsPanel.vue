<script setup lang="ts">
import type { ApiInput } from '@shared/domain/dbmcp';
import Disclosure from '@theme/components/Disclosure.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import SecretInput from '@workbench/components/SecretInput.vue';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed, ref, watchEffect } from 'vue';

// The values the agent asked the person to type: one row each (label, masked field, use once or keep).
// Needed ones block Send; a stored one (held from an earlier answer, same origin) may be replaced. A
// typed value lives only in the approvals store until the answer goes out. What the choice means is
// a tooltip on its two items, never a line per row.
const props = defineProps<{ itemId: string; inputs: readonly ApiInput[] }>();

const store = useApprovalsStore();
const needed = computed(() => props.inputs.filter((i) => i.state === 'needed'));
const stored = computed(() => props.inputs.filter((i) => i.state === 'stored'));

// The card starts on the agent's suggestion; the person's pick wins.
watchEffect(() => {
  for (const i of props.inputs) {
    if (store.inputModes[props.itemId]?.[i.name] === undefined) {
      store.setInputMode(props.itemId, i.name, i.mode);
    }
  }
});

const typed = (name: string): string => store.inputs[props.itemId]?.[name] ?? '';
// Spaces only: the store drops it as no value, so the field is marked instead of showing an error line.
const spaces = ref<Record<string, boolean>>({});
function onType(name: string, v: string): void {
  spaces.value = { ...spaces.value, [name]: v.length > 0 && v.trim() === '' };
  store.setInput(props.itemId, name, v);
}
const fieldId = (name: string): string => `api-input-${props.itemId}-${name}`;
const mode = (i: ApiInput): 'once' | 'keep' => store.inputModes[props.itemId]?.[i.name] ?? i.mode;
const choices = (i: ApiInput) => [
  {
    value: 'once',
    label: t('studio.kept.input.once.label'),
    tooltip: t('studio.kept.input.once.tooltip'),
    testid: `api-input-${i.name}-once`,
  },
  {
    value: 'keep',
    label: t('studio.kept.input.keep.label', { minutes: i.keepMinutes }),
    tooltip: t('studio.kept.input.keep.tooltip', { origin: i.origin }),
    testid: `api-input-${i.name}-keep`,
  },
];
</script>

<template>
  <section
    v-if="needed.length || stored.length"
    class="flex flex-col gap-2"
    data-testid="api-approval-inputs"
  >
    <div
      v-for="i in needed"
      :key="i.name"
      class="flex flex-wrap items-center gap-x-2 gap-y-1"
      :data-testid="`api-approval-input-${i.name}`"
    >
      <FieldLabel :for="fieldId(i.name)" class="shrink-0">{{ i.label }}</FieldLabel>
      <SecretInput
        :id="fieldId(i.name)"
        class="min-w-48 flex-1"
        :model-value="typed(i.name)"
        :placeholder="t('common.required.label')"
        :aria-invalid="spaces[i.name] || undefined"
        :data-testid="`api-input-${i.name}`"
        @update:model-value="(v: string) => onType(i.name, v)"
      />
      <SecondaryTabs
        variant="choice"
        :aria-label="t('studio.kept.input.choice.label')"
        :items="choices(i)"
        :model-value="mode(i)"
        @update:model-value="
          (v: string) => store.setInputMode(itemId, i.name, v === 'keep' ? 'keep' : 'once')
        "
      />
    </div>
    <Disclosure
      v-for="i in stored"
      :key="i.name"
      meta
      :label="t('studio.kept.input.replace.label', { label: i.label })"
      :data-testid="`api-approval-input-replace-${i.name}`"
    >
      <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
        <SecretInput
          :id="fieldId(i.name)"
          class="min-w-48 flex-1"
          :model-value="typed(i.name)"
          :aria-label="i.label"
          :data-testid="`api-input-${i.name}`"
          @update:model-value="(v: string) => onType(i.name, v)"
        />
        <SecondaryTabs
          variant="choice"
          :aria-label="t('studio.kept.input.choice.label')"
          :items="choices(i)"
          :model-value="mode(i)"
          @update:model-value="
            (v: string) => store.setInputMode(itemId, i.name, v === 'keep' ? 'keep' : 'once')
          "
        />
      </div>
    </Disclosure>
  </section>
</template>
