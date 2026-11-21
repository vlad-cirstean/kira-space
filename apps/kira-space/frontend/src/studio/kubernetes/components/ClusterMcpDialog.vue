<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
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
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { wrapSelectionOnType } from '@theme/wrapSelection';
import { useMutationAction } from '@workbench/behaviour/asyncAction';
import { computed, ref, useId, watch } from 'vue';
import { useSettingsStore } from '../../../state/settings';
import ReviewChoiceField from '../../components/ReviewChoiceField.vue';
import { useClusters, useSetClusterMcp } from '../queries';
import type { KubeMcpSettings } from '../wire';

const props = defineProps<{ clusterId: string }>();
const open = defineModel<boolean>('open', { required: true });

const clusters = useClusters();
const settingsStore = useSettingsStore();
const cluster = computed(() => clusters.data.value?.clusters.find((c) => c.id === props.clusterId));
const saving = useMutationAction(useSetClusterMcp());

const approvalItems = computed(() => [
  { value: 'prompt', label: t('common.mcp.askFirst.label'), testid: 'kube-mcp-prompt' },
  { value: 'allow', label: t('kubernetes.mcp.allow.label'), testid: 'kube-mcp-allow' },
]);

const draft = ref<KubeMcpSettings | null>(null);
const dirty = computed(
  () => !!draft.value && JSON.stringify(draft.value) !== JSON.stringify(cluster.value?.mcp),
);

watch(
  [open, cluster],
  ([isOpen, current]) => {
    if (isOpen && current && !draft.value) draft.value = { ...current.mcp };
    if (!isOpen) {
      draft.value = null;
      saving.reset();
    }
  },
  { immediate: true },
);

async function onSave(): Promise<void> {
  if (!draft.value) return;
  if (await saving.run({ clusterId: props.clusterId, mcp: draft.value })) open.value = false;
}
const idBase = useId();
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent size="lg" :dirty="dirty" data-testid="kube-mcp-dialog">
      <DialogHeader close-testid="kube-mcp-close">
        <DialogTitle>{{
          t('kubernetes.mcp.dialog.title', { cluster: cluster?.context ?? '' })
        }}</DialogTitle>
        <DialogDescription>{{ t('kubernetes.mcp.dialog.description') }}</DialogDescription>
      </DialogHeader>
      <DialogBody v-if="draft">
        <Field orientation="horizontal">
          <Checkbox
            id="kube-mcp-enabled"
            :model-value="draft.enabled"
            data-testid="kube-mcp-enabled"
            @update:model-value="(v) => (draft!.enabled = v === true)"
          />
          <FieldLabel for="kube-mcp-enabled">{{ t('kubernetes.mcp.enable.label') }}</FieldLabel>
        </Field>
        <FieldDescription>{{ t('kubernetes.mcp.enable.hint') }}</FieldDescription>
        <template v-if="draft.enabled">
          <Field>
            <FieldLabel :for="`${idBase}-kube-mcp-description`">{{
              t('common.field.description.label')
            }}</FieldLabel>
            <Textarea
              :id="`${idBase}-kube-mcp-description`"
              class="font-data"
              v-model="draft.description"
              maxlength="1000"
              data-testid="kube-mcp-description"
              @keydown="wrapSelectionOnType"
            />
            <FieldDescription>{{ t('kubernetes.mcp.descriptionNote.label') }}</FieldDescription>
          </Field>
          <Field>
            <FieldLabel>{{ t('kubernetes.mcp.approval.label') }}</FieldLabel>
            <SecondaryTabs
              variant="choice"
              size="kira-lg"
              :aria-label="t('kubernetes.mcp.approval.label')"
              :items="approvalItems"
              :model-value="draft.approval"
              data-testid="kube-mcp-approval"
              @update:model-value="(v) => (draft!.approval = v === 'allow' ? 'allow' : 'prompt')"
            />
          </Field>
          <ReviewChoiceField
            v-model="draft.review"
            :label="t('kubernetes.mcp.review.label')"
            :app-on="settingsStore.dbMcp.kubeReviewResponses"
            testid="kube-mcp-review"
          />
          <FieldDescription>{{ t('kubernetes.mcp.review.hint') }}</FieldDescription>
        </template>
        <Alert v-if="saving.error.value" variant="error" data-testid="kube-mcp-error">
          <AlertDescription>{{ saving.error.value }}</AlertDescription>
        </Alert>
      </DialogBody>
      <DialogFooter>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :pending="saving.pending.value"
          :disabled="!draft"
          data-testid="kube-mcp-save"
          @click="onSave"
        >
          {{ t('common.action.save.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
