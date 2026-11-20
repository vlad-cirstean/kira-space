<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
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
import { FieldLegend, FieldSet } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import Identifier from '@workbench/components/Identifier.vue';
import type { PendingChange } from '../lib/editDiff';
import type { DockerEditSpec } from '../wire';
import EditPendingSummary from './EditPendingSummary.vue';

defineProps<{ spec: DockerEditSpec; changes: PendingChange[]; pending: boolean }>();
const emit = defineEmits<{ close: []; confirm: [] }>();

const COMPOSE = 'Compose';
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent size="lg" destructive data-testid="docker-edit-dialog">
      <DialogHeader close-testid="docker-edit-close">
        <DialogTitle>{{ t('docker.dialog.recreateConfirm.title') }}</DialogTitle>
        <DialogDescription>{{ t('docker.dialog.recreateConfirm.body') }}</DialogDescription>
      </DialogHeader>

      <DialogBody>
        <EditPendingSummary :changes="changes" />

        <FieldSet data-testid="docker-edit-lost">
          <FieldLegend>{{ t('docker.dialog.recreateConfirm.lost.title') }}</FieldLegend>
          <ul class="m-0 flex list-disc flex-col gap-0.5 pl-5">
            <li data-testid="docker-edit-lost-layer">{{
              t('docker.dialog.recreateConfirm.lostLayer.body')
            }}</li>
            <li
              v-for="v in spec.anonymousVolumes"
              :key="v.name"
              class="break-all"
              data-testid="docker-edit-lost-volume"
            >
              <CatalogText k="docker.dialog.recreateConfirm.lostVolume.body">
                <template #name><Identifier :value="v.name" /></template>
                <template #destination><Identifier :value="v.destination" /></template>
              </CatalogText>
            </li>
            <li>{{ t('docker.dialog.recreateConfirm.lostLogs.body') }}</li>
            <li>{{ t('docker.dialog.recreateConfirm.lostId.body') }}</li>
          </ul>
        </FieldSet>

        <FieldSet data-testid="docker-edit-kept">
          <FieldLegend>{{ t('docker.dialog.recreateConfirm.kept.title') }}</FieldLegend>
          <ul class="m-0 flex list-disc flex-col gap-0.5 pl-5">
            <li>{{ t('docker.dialog.recreateConfirm.keptBase.body') }}</li>
            <li v-if="spec.preserved.length">{{
              t('docker.dialog.recreateConfirm.keptMore.body', { items: spec.preserved.join(', ') })
            }}</li>
            <li v-if="spec.state === 'running'">{{
              t('docker.dialog.recreateConfirm.restarts.body')
            }}</li>
            <li v-else>{{ t('docker.dialog.recreateConfirm.staysStopped.body') }}</li>
          </ul>
        </FieldSet>

        <Alert v-if="spec.dependents.length" variant="warn" data-testid="docker-edit-dependents">
          <AlertDescription>
            {{
              t('docker.dialog.recreateConfirm.dependents.body', {
                names: spec.dependents.join(', '),
              })
            }}
          </AlertDescription>
        </Alert>
        <Alert v-if="spec.origin === 'compose'" variant="info" data-testid="docker-edit-compose">
          <AlertDescription>
            {{
              t('docker.dialog.recreateConfirm.compose.body', {
                tool: COMPOSE,
                name: spec.originName,
              })
            }}
          </AlertDescription>
        </Alert>
      </DialogBody>

      <DialogFooter>
        <Button
          variant="dialog-danger"
          size="kira-lg"
          :pending="pending"
          data-testid="docker-edit-confirm"
          @click="emit('confirm')"
        >
          {{ t('docker.dialog.recreateConfirm.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
