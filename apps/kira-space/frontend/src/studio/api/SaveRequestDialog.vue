<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { Field, FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { useFormErrors } from '@workbench/behaviour/formErrors';
import { computed, ref, watch } from 'vue';
import { useCollectionsStore } from './state/collections';
import { useSaveRequestDialogStore } from './state/saveRequestDialog';

const collectionsStore = useCollectionsStore();
const saveDialogStore = useSaveRequestDialogStore();

// Save as… — one text field for the name and one indented native select of every collection and
// folder as the target, on the existing DialogFrame. Driven by useSaveRequestDialogStore's own
// open/suggestedName state so the request view can open it without importing this
// component (the same shape state/objectStore.ts's own upload dialog uses).
const name = ref('');
const target = ref('');
const {
  run: runSave,
  pending: saving,
  error,
} = useAsyncAction(async (collectionId: string, parentId: string | null, trimmed: string) => {
  const failure = await collectionsStore.submitSaveDialog(collectionId, parentId, trimmed);
  if (failure !== null) throw new Error(failure);
});

/** Every collection, each carrying its own folders as a real <optgroup> (F7) — the value encodes
 *  both halves because a folder id alone does not say which collection it belongs to. */
const collectionTargets = computed(() =>
  collectionsStore.collections.map((collection) => ({
    id: collection.id,
    name: collection.name,
    folders: collectionsStore.folderPaths(collection.id),
  })),
);

const firstTargetValue = computed(() => {
  const first = collectionTargets.value[0];
  return first ? `${first.id}:` : '';
});

const { errorFor, errorId, invalidAttrs, submit, reset } = useFormErrors({
  name: {
    value: () => name.value,
    error: () => (name.value.trim() ? null : t('studio.api.saveRequest.nameRequired.error')),
  },
});

const dirty = computed(() => name.value !== saveDialogStore.suggestedName);

watch(
  () => saveDialogStore.open,
  (open) => {
    if (!open) return;
    name.value = saveDialogStore.suggestedName;
    target.value = firstTargetValue.value;
    error.value = null;
    reset();
  },
  { immediate: true },
);

async function onSave(): Promise<void> {
  if (saving.value) return;
  if (!(await submit()) || !target.value) return;
  const trimmed = name.value.trim();
  const [collectionId, parentId] = splitTarget(target.value);
  await runSave(collectionId, parentId, trimmed);
}

function splitTarget(value: string): [string, string | null] {
  const idx = value.indexOf(':');
  const collectionId = value.slice(0, idx);
  const folderId = value.slice(idx + 1);
  return [collectionId, folderId === '' ? null : folderId];
}
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && saveDialogStore.closeSaveDialog()">
    <DialogContent size="md" :dirty="dirty" data-testid="save-request-dialog">
      <DialogHeader close-testid="save-request-close">
        <DialogTitle>{{ t('studio.api.saveRequest.title') }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <Field>
          <Label class="text-muted-foreground">{{ t('common.field.name.label') }}</Label>
          <Input
            v-model="name"
            v-bind="invalidAttrs('name')"
            data-testid="save-request-name"
            @keydown.enter="onSave"
          />
          <FieldError
            v-if="errorFor('name')"
            :id="errorId('name')"
            data-testid="save-request-name-error"
            >{{
              errorFor('name')
            }}</FieldError
          >
        </Field>

        <Field>
          <Label class="text-muted-foreground">{{
            t('studio.api.saveRequest.target.label')
          }}</Label>
          <NativeSelect v-model="target" variant="bordered" data-testid="save-request-target">
            <optgroup v-for="c in collectionTargets" :key="c.id" :label="c.name">
              <option :value="`${c.id}:`">{{
                t('studio.api.saveRequest.collectionRoot.option')
              }}</option>
              <option v-for="f in c.folders" :key="f.id" :value="`${c.id}:${f.id}`">{{
                f.label
              }}</option>
            </optgroup>
          </NativeSelect>
        </Field>

        <Alert
          v-if="collectionTargets.length === 0"
          variant="warn"
          data-testid="save-request-no-target"
        >
          <AlertDescription>
            {{ t('studio.api.saveRequest.noTarget.body') }}
          </AlertDescription>
        </Alert>
        <Alert v-if="error" variant="error" data-testid="save-request-error">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
      </DialogBody>

      <DialogFooter>
        <Button
          variant="dialog"
          size="kira-lg"
          data-testid="save-request-cancel"
          @click="saveDialogStore.closeSaveDialog"
          >{{
            t('common.action.cancel.label')
          }}</Button
        >
        <Button
          variant="dialog-primary"
          size="kira-lg"
          data-testid="save-request-submit"
          :disabled="!target"
          :pending="saving"
          @click="onSave"
        >
          {{ t('common.action.save.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
