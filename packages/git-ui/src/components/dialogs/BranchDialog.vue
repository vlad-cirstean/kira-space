<script setup lang="ts">
/**
 * "Create branch here" (the row menu). Branch creation has no comparable hazard (git never refuses
 * it, and there is no annotated-tag-style silent-data-loss case to guard), so this file is the
 * same small name/start-point/checkout-toggle modal `TagDialog.vue` uses for a tag, reusing
 * `core`'s own `validateRefName` prefilter (branch and tag names share the same
 * `check-ref-format --branch` rule).
 *
 * The modal shell is shadcn's `Dialog`/`DialogContent` now — this file still only supplies its own
 * body/footer content.
 */
import { shortSha, validateRefName } from '@kira/git-core';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
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

const props = defineProps<{ open: boolean; startPoint: string; ops: OpsState }>();
const emit = defineEmits<(e: 'close') => void>();

const name = ref('');
const checkout = ref(true);
const nameId = useId();
const checkoutId = useId();

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    name.value = '';
    checkout.value = true;
    void nextTick(form.reset);
  },
);

const nameError = computed(() => {
  if (name.value === '') return undefined;
  const { valid, error } = validateRefName(name.value);
  return valid || error === undefined ? undefined : renderText(error);
});
const canSubmit = computed(() => name.value !== '' && nameError.value === undefined);
const form = useFormErrors({
  name: { value: () => name.value, error: () => nameError.value ?? null },
});

function cancel(): void {
  emit('close');
}

async function submit(): Promise<void> {
  if (!canSubmit.value) return;
  const result = await props.ops.branchCreate({
    name: name.value,
    startPoint: props.startPoint,
    checkout: checkout.value,
    track: undefined,
  });
  if (!result.ok) return;
  emit('close');
}
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && cancel()">
    <DialogContent size="md" :dirty="name !== ''">
      <DialogHeader>
        <DialogTitle>{{ t('git.dialog.branch.title') }}</DialogTitle>
        <DialogDescription>
          <CatalogText k="git.dialog.branch.startPoint.description">
            <template #sha><Identifier :value="startPoint" :short="shortSha" /></template>
          </CatalogText>
        </DialogDescription>
      </DialogHeader>
      <DialogBody>
        <Field>
          <FieldLabel :for="nameId">{{ t('common.field.name.label') }}</FieldLabel>
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

        <Field orientation="horizontal">
          <Checkbox :id="checkoutId" v-model="checkout" />
          <FieldLabel :for="checkoutId">{{ t('git.dialog.branch.checkout.label') }}</FieldLabel>
        </Field>
      </DialogBody>

      <DialogFooter>
        <Button variant="dialog-primary" size="kira-lg" :disabled="!canSubmit" @click="submit">
          {{ t('common.action.create.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
