<script setup lang="ts">
import type { ApiInput } from '@shared/domain/dbmcp';
import Disclosure from '@theme/components/Disclosure.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import SecretInput from '@workbench/components/SecretInput.vue';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed } from 'vue';

// The values the agent asked the person to type. Needed ones block Approve; a stored one (held from
// an earlier answer, same origin) may be replaced. A typed value lives only in the approvals store
// until the answer goes out, and nothing here echoes a stored value.
const props = defineProps<{ itemId: string; inputs: readonly ApiInput[] }>();

const store = useApprovalsStore();
const origin = computed(() => props.inputs[0]?.origin ?? '');
const needed = computed(() => props.inputs.filter((i) => i.state === 'needed'));
const stored = computed(() => props.inputs.filter((i) => i.state === 'stored'));

const typed = (name: string): string => store.inputs[props.itemId]?.[name] ?? '';
const fieldId = (name: string): string => `api-input-${props.itemId}-${name}`;
</script>

<template>
  <section class="flex flex-col gap-1" data-testid="api-approval-inputs">
    <SectionHeading flush :label="t('studio.kept.input.title.label')" />
    <FieldDescription data-testid="api-approval-inputs-hint">{{
      t('studio.kept.input.hint', { origin })
    }}</FieldDescription>
    <Field v-for="i in needed" :key="i.name" :data-testid="`api-approval-input-${i.name}`">
      <FieldLabel :for="fieldId(i.name)" required>{{ i.label }}</FieldLabel>
      <SecretInput
        :id="fieldId(i.name)"
        :model-value="typed(i.name)"
        :data-testid="`api-input-${i.name}`"
        @update:model-value="(v: string) => store.setInput(itemId, i.name, v)"
      />
    </Field>
    <Disclosure
      v-for="i in stored"
      :key="i.name"
      meta
      :label="t('studio.kept.input.replace.label', { label: i.label })"
      :data-testid="`api-approval-input-replace-${i.name}`"
    >
      <Field>
        <FieldLabel :for="fieldId(i.name)">{{ i.label }}</FieldLabel>
        <SecretInput
          :id="fieldId(i.name)"
          :model-value="typed(i.name)"
          :data-testid="`api-input-${i.name}`"
          @update:model-value="(v: string) => store.setInput(itemId, i.name, v)"
        />
      </Field>
    </Disclosure>
  </section>
</template>
