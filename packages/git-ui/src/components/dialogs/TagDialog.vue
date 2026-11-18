<script setup lang="ts">
import { shortSha } from '@kira/git-core';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
/**
 * "create tag here" (the row menu). Unlike `CheckoutDialog.vue`/`RevertDialog.vue`, this one is not
 * driven by an `OpsState` pending-ref (there is no `preflight.tagCreate` endpoint) — `App.vue`
 * opens it directly with the target sha and closes it on `close`; `tagDialogModel.ts` supplies the
 * pure classification either way.
 *
 * The modal shell is shadcn's `Dialog`/`DialogContent` now — this file still
 * only supplies its own body/footer content.
 */

import type { RefRow } from '@kira/git-ipc';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
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
import { Textarea } from '@theme/components/ui/textarea';
import { useFormErrors } from '@workbench/behaviour/formErrors';
import Identifier from '@workbench/components/Identifier.vue';
import { computed, nextTick, ref, useId, watch } from 'vue';
import { canSubmitTagCreate, classifyTagName } from '../../lib/tagDialogModel.ts';
import { renderText } from '../../lib/text.ts';
import type { OpsState } from '../../state/ops.ts';

const props = defineProps<{
  open: boolean;
  target: string;
  existingTags: readonly RefRow[];
  ops: OpsState;
}>();

const emit = defineEmits<(e: 'close') => void>();

const name = ref('');
const annotated = ref(false);
const message = ref('');
const force = ref(false);
const nameId = useId();
const forceId = useId();
const annotatedId = useId();
const messageId = useId();

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    name.value = '';
    annotated.value = false;
    message.value = '';
    force.value = false;
    void nextTick(form.reset);
  },
);

const state = computed(() => classifyTagName(name.value, props.existingTags, force.value));
const canSubmit = computed(() => canSubmitTagCreate(state.value, annotated.value, message.value));
const form = useFormErrors({
  name: {
    value: () => name.value,
    error: () => (state.value.nameError ? renderText(state.value.nameError) : null),
  },
});

function cancel(): void {
  emit('close');
}

async function submit(): Promise<void> {
  if (!canSubmit.value) return;
  const result = await props.ops.tagCreate({
    name: name.value,
    target: props.target,
    message: annotated.value ? message.value : undefined,
    force: force.value,
  });
  if (!result.ok) return;
  emit('close');
}
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && cancel()">
    <DialogContent size="md" :dirty="name !== '' || message !== ''">
      <DialogHeader>
        <DialogTitle>{{ t('git.dialog.tag.title') }}</DialogTitle>
        <DialogDescription>
          <CatalogText k="git.dialog.tag.target.description">
            <template #sha><Identifier :value="target" :short="shortSha" /></template>
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

        <template v-if="state.verdict === 'blockedByExisting'">
          <Alert variant="warn">
            <AlertDescription>
              {{
                t(
                  state.existingIsAnnotated
                    ? 'git.dialog.tag.existsAnnotated.hint'
                    : 'git.dialog.tag.exists.hint',
                  { name },
                )
              }}
            </AlertDescription>
          </Alert>
          <Field orientation="horizontal">
            <Checkbox :id="forceId" v-model="force" />
            <FieldLabel :for="forceId">{{ t('git.dialog.tag.replace.label') }}</FieldLabel>
          </Field>
        </template>

        <Alert
          v-if="state.verdict === 'movesWithForce' && state.requiresAnnotationToPreserve"
          variant="warn"
        >
          <AlertDescription>
            {{ t('git.dialog.tag.downgrade.hint') }}
          </AlertDescription>
        </Alert>

        <Field orientation="horizontal">
          <Checkbox :id="annotatedId" v-model="annotated" />
          <FieldLabel :for="annotatedId">{{ t('git.tag.annotated.status') }}</FieldLabel>
        </Field>
        <Field v-if="annotated">
          <FieldLabel :for="messageId">{{ t('common.message.label') }}</FieldLabel>
          <Textarea :id="messageId" v-model="message" />
        </Field>
      </DialogBody>

      <DialogFooter>
        <Button variant="dialog" size="kira-lg" @click="cancel">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button variant="dialog-primary" size="kira-lg" :disabled="!canSubmit" @click="submit">
          {{ t('common.action.create.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
