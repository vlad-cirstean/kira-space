<script setup lang="ts">
import type { ApiInput } from '@shared/domain/dbmcp';
import Disclosure from '@theme/components/Disclosure.vue';
import { Button } from '@theme/components/ui/button';
import { FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import SecretInput from '@workbench/components/SecretInput.vue';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed, ref } from 'vue';
import { useSettingsStore } from '../../state/settings';

// The values the agent asked the person to type: one row each (label, masked field). Rows share one
// grid, so the fields line up whatever the label. Every typed value is kept; one line under the rows
// says for how long and opens the retention setting. Needed ones block Send; a stored one (held from
// an earlier answer, same origin) may be replaced. A typed value lives only in the approvals store
// until the answer goes out.
const props = defineProps<{ itemId: string; inputs: readonly ApiInput[] }>();

// Trigger spans the row; the content joins the panel grid so its field lines up with the others.
const STORED_ROW =
  'col-span-full grid grid-cols-subgrid [&>[data-slot=collapsible-trigger]]:col-span-full [&>[data-slot=collapsible-content]]:col-span-full [&>[data-slot=collapsible-content]]:grid [&>[data-slot=collapsible-content]]:grid-cols-subgrid [&>[data-slot=collapsible-content]]:items-center';

const store = useApprovalsStore();
const settings = useSettingsStore();
const needed = computed(() => props.inputs.filter((i) => i.state === 'needed'));
const stored = computed(() => props.inputs.filter((i) => i.state === 'stored'));
// One setting for every value, so the first input's minutes stand for all.
const keepMinutes = computed(() => props.inputs[0]?.keepMinutes ?? 60);

const typed = (name: string): string => store.inputs[props.itemId]?.[name] ?? '';
// Spaces only: the store drops it as no value, so the field is marked instead of showing an error line.
const spaces = ref<Record<string, boolean>>({});
function onType(name: string, v: string): void {
  spaces.value = { ...spaces.value, [name]: v.length > 0 && v.trim() === '' };
  store.setInput(props.itemId, name, v);
}
const fieldId = (name: string): string => `api-input-${props.itemId}-${name}`;
</script>

<template>
  <section
    v-if="needed.length || stored.length"
    class="grid grid-cols-[minmax(0,max-content)_minmax(12rem,1fr)] items-center gap-x-2 gap-y-2"
    data-testid="api-approval-inputs"
  >
    <div
      v-for="i in needed"
      :key="i.name"
      class="col-span-full grid grid-cols-subgrid items-center"
      :data-testid="`api-approval-input-${i.name}`"
    >
      <FieldLabel :for="fieldId(i.name)" class="max-w-48 break-words font-data">{{
        i.name
      }}</FieldLabel>
      <SecretInput
        :id="fieldId(i.name)"
        class="min-w-0"
        :model-value="typed(i.name)"
        :placeholder="t('common.required.label')"
        :aria-invalid="spaces[i.name] || undefined"
        :data-testid="`api-input-${i.name}`"
        @update:model-value="(v: string) => onType(i.name, v)"
      />
    </div>
    <Disclosure
      v-for="i in stored"
      :key="i.name"
      meta
      :class="STORED_ROW"
      :label="t('studio.kept.input.replace.label', { label: i.name })"
      :data-testid="`api-approval-input-replace-${i.name}`"
    >
      <span aria-hidden="true" />
      <SecretInput
        :id="fieldId(i.name)"
        class="min-w-0"
        :model-value="typed(i.name)"
        :aria-label="i.name"
        :data-testid="`api-input-${i.name}`"
        @update:model-value="(v: string) => onType(i.name, v)"
      />
    </Disclosure>
    <FieldDescription class="col-span-full">
      <Button
        variant="link-meta"
        size="inline"
        data-testid="api-approval-inputs-kept-for"
        @click="settings.openSettingsAt('Studio MCP variables')"
      >
        {{ t('studio.kept.input.keptFor.label', { minutes: keepMinutes }) }}
      </Button>
    </FieldDescription>
  </section>
</template>
