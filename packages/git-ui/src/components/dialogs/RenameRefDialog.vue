<script setup lang="ts">
/**
 * "Rename branch…" from the graph's own ref-badge context menu
 * (`App.vue`'s ref menu). `RefsPopover.vue`'s own rename swaps its dropdown row
 * for an inline text field — there is no comparable row here (the badge is a DOM fragment inside
 * a commit's message cell, not a list item with room to become an input), so this is a small
 * modal instead: `BranchDialog.vue`'s own "no comparable hazard" judgment call (git never
 * refuses a rename the way it can refuse a checkout or a force-move), extended to a rename with
 * a prefilled name field rather than an empty one, validated with the same `validateRefName`
 * prefilter `BranchDialog.vue`/`TagDialog.vue` already use.
 *
 * The modal shell is shadcn's `Dialog`/`DialogContent` now — this file still
 * only supplies its own body/footer content.
 */
import { validateRefName } from '@kira/git-core';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { Field, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { useFormErrors } from '@workbench/behaviour/formErrors';
import Identifier from '@workbench/components/Identifier.vue';
import { computed, nextTick, ref, useId, watch } from 'vue';
import { renderText } from '../../lib/text.ts';
import type { OpsState } from '../../state/ops.ts';

const props = defineProps<{ open: boolean; currentName: string; ops: OpsState }>();
const emit = defineEmits<(e: 'close') => void>();

const name = ref('');
const nameId = useId();

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    name.value = props.currentName;
    void nextTick(form.reset);
  },
);

const nameError = computed(() => {
  if (name.value === '' || name.value === props.currentName) return undefined;
  const { valid, error } = validateRefName(name.value);
  return valid || error === undefined ? undefined : renderText(error);
});
const canSubmit = computed(
  () => name.value !== '' && name.value !== props.currentName && nameError.value === undefined,
);
const form = useFormErrors({
  name: { value: () => name.value, error: () => nameError.value ?? null },
});

function cancel(): void {
  emit('close');
}

async function submit(): Promise<void> {
  if (!canSubmit.value) return;
  const result = await props.ops.branchRename(props.currentName, name.value);
  if (!result.ok) return;
  emit('close');
}
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && cancel()">
    <DialogContent size="md" :dirty="name !== currentName">
      <DialogHeader>
        <DialogTitle>{{ t('git.dialog.rename.title') }}</DialogTitle>
        <DialogDescription>
          <CatalogText k="git.dialog.rename.description">
            <template #name><Identifier :value="currentName" /></template>
          </CatalogText>
        </DialogDescription>
      </DialogHeader>
      <DialogBody>
        <Field>
          <FieldLabel :for="nameId">{{ t('git.dialog.rename.name.label') }}</FieldLabel>
          <Input
            :id="nameId"
            v-model="name"
            type="text"
            class="w-full"
            v-bind="form.invalidAttrs('name')"
          />
          <FieldError v-if="form.errorFor('name')" :id="form.errorId('name')">{{
            form.errorFor('name')
          }}</FieldError>
        </Field>
      </DialogBody>

      <DialogFooter>
        <Button variant="dialog" size="kira-lg" @click="cancel">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button variant="dialog-primary" size="kira-lg" :disabled="!canSubmit" @click="submit">
          {{ t('common.action.rename.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
