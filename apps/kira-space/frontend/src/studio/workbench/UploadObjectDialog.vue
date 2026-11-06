<script setup lang="ts">
import { contentTypeForFilename } from '@shared/domain/objectStore';
import { decodePath } from '@shared/domain/tree';
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
import { Field } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { formatBytes } from '@workbench/behaviour/format';
import { computed, ref, watch } from 'vue';
import { control } from '../bridge/control';
import { useObjectStoreStore } from '../state/objectStore';
import { useStaleDataStore } from '../state/staleData';
import { useTabsStore } from '../state/tabs';
import { browseInvalidate } from '../state/viewCommands';

const objectStoreStore = useObjectStoreStore();
const tabsStore = useTabsStore();

// Three entry points (the Browse panel's own container rows/toolbar and — until
// the tree stops rendering bucket/prefix rows — the tree's own bucket/prefix menu) — driven
// entirely by state/objectStore.ts's uploadDialogState so project/menus.ts can open it without
// importing this component or any views/ module (the dependency rule).

const chosenFile = ref<{ path: string; name: string; size: number } | null>(null);
const key = ref('');
const contentType = ref('');
const {
  run: runUpload,
  pending: saving,
  error,
} = useAsyncAction(async (connectionId: string, file: { path: string }) => {
  const newPath = await objectStoreStore.uploadObject({
    connectionId,
    containerPath: objectStoreStore.containerPath,
    key: key.value.trim(),
    sourcePath: file.path,
    contentType: contentType.value.trim() || 'application/octet-stream',
    tabId: null,
  });
  browseInvalidate(connectionId, objectStoreStore.containerPath);
  objectStoreStore.closeUploadDialog();
  tabsStore.openKeyValueTab(connectionId, newPath, { newTab: true });
});

// Every ancestor `prefix` segment of the container path, joined — not just the last
// one. A bucket has none; a one-level-nested container has one; `bucket/prefix:a/prefix:b` joins
// to `a/b/`. The old `pathTail`-only version silently dropped every segment above the immediate
// parent, so an upload two or more levels deep prefilled a key that was missing its own ancestry
// and landed at the wrong place in the bucket. Mirrors `s3/catalog.ts`'s own
// `prefixSegments.join('/') + '/'` reconstruction on the engine side, so the two agree by
// construction rather than by coincidence.
const containerPrefix = computed(() => {
  const { connectionId, containerPath } = objectStoreStore;
  if (!connectionId || containerPath === '') return '';
  const prefixes = decodePath(connectionId, containerPath).segments.filter(
    (s) => s.kind === 'prefix',
  );
  return prefixes.length > 0 ? `${prefixes.map((s) => s.name).join('/')}/` : '';
});

async function chooseFile(): Promise<void> {
  const res = await control.filesChooseOpen();
  if (res.canceled || !res.file) return;
  chosenFile.value = res.file;
  key.value = `${containerPrefix.value}${res.file.name}`;
  contentType.value = contentTypeForFilename(res.file.name);
  error.value = null;
}

function onClose(): void {
  objectStoreStore.closeUploadDialog();
}

async function onUpload(): Promise<void> {
  const connectionId = objectStoreStore.connectionId;
  const file = chosenFile.value;
  if (!connectionId || !file || !key.value.trim()) return;
  const originTab = objectStoreStore.tabId;
  if (originTab && useStaleDataStore().isStale(originTab, originTab, connectionId)) {
    error.value = t('studio.common.connectionLost.error');
    return;
  }
  await runUpload(connectionId, file);
}

// Reset every time the dialog opens — a stale chosen file from a previous open must never carry
// over to a different bucket/prefix.
watch(
  () => objectStoreStore.open,
  (open) => {
    if (!open) return;
    chosenFile.value = null;
    key.value = '';
    contentType.value = '';
    error.value = null;
  },
);
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && onClose()">
    <DialogContent size="md" :dirty="chosenFile !== null" data-testid="upload-dialog">
      <DialogHeader close-testid="upload-close">
        <DialogTitle>{{ t('studio.keyValue.upload.tooltip') }}</DialogTitle>
      </DialogHeader>

      <DialogBody>
        <Button
          variant="dialog"
          size="kira-lg"
          class="self-start"
          data-testid="upload-choose-file"
          @click="chooseFile"
        >
          {{ t('studio.http.body.chooseFile.label') }}
        </Button>
        <div v-if="chosenFile" :class="rowMeta" data-testid="upload-chosen-file">
          {{ chosenFile.name }}
          ({{ formatBytes(chosenFile.size) }})
        </div>

        <template v-if="chosenFile">
          <Field>
            <Label class="text-muted-foreground p-0">{{ t('common.field.key.label') }}</Label>
            <Input v-model="key" class="font-data" data-testid="upload-key" />
          </Field>

          <Field>
            <Label class="text-muted-foreground p-0">{{
              t('studio.http.body.contentType.placeholder')
            }}</Label>
            <Input v-model="contentType" class="font-data" data-testid="upload-content-type" />
          </Field>
        </template>

        <Alert v-if="error" variant="error" data-testid="upload-error">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
      </DialogBody>

      <DialogFooter>
        <Button variant="dialog" size="kira-lg" data-testid="upload-cancel" @click="onClose">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          data-testid="upload-submit"
          :disabled="!chosenFile || !key.trim()"
          :pending="saving"
          @click="onUpload"
        >
          {{ t('studio.upload.submit.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
