<script setup lang="ts">
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
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@theme/components/ui/radio-group';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { wrapSelectionOnType } from '@theme/wrapSelection';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref, watch } from 'vue';
import { useClusters, useSetClusterMcp } from '../queries';
import type { KubeMcpSettings } from '../wire';

const props = defineProps<{ clusterId: string }>();
const open = defineModel<boolean>('open', { required: true });

const clusters = useClusters();
const cluster = computed(() => clusters.data.value?.clusters.find((c) => c.id === props.clusterId));
const save = useSetClusterMcp();

const draft = ref<KubeMcpSettings | null>(null);
const error = ref<string | null>(null);
const dirty = computed(
  () => !!draft.value && JSON.stringify(draft.value) !== JSON.stringify(cluster.value?.mcp),
);

watch(
  [open, cluster],
  ([isOpen, current]) => {
    if (isOpen && current && !draft.value) draft.value = { ...current.mcp };
    if (!isOpen) {
      draft.value = null;
      error.value = null;
    }
  },
  { immediate: true },
);

function onSave(): void {
  if (!draft.value) return;
  error.value = null;
  save.mutate(
    { clusterId: props.clusterId, mcp: draft.value },
    {
      onSuccess: () => {
        open.value = false;
      },
      onError: (err) => {
        error.value = errorText(err);
      },
    },
  );
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent size="lg" :dirty="dirty" data-testid="kube-mcp-dialog">
      <DialogHeader>
        <DialogTitle>{{
          t('kubernetes.mcp.dialog.title', { cluster: cluster?.context ?? '' })
        }}</DialogTitle>
        <DialogDescription>{{ t('kubernetes.mcp.dialog.description') }}</DialogDescription>
      </DialogHeader>
      <DialogBody v-if="draft" class="flex flex-col gap-3">
        <Field orientation="horizontal">
          <Checkbox
            id="kube-mcp-enabled"
            :model-value="draft.enabled"
            data-testid="kube-mcp-enabled"
            @update:model-value="(v) => (draft!.enabled = v === true)"
          />
          <Label for="kube-mcp-enabled">{{ t('kubernetes.mcp.enable.label') }}</Label>
        </Field>
        <FieldDescription>{{ t('kubernetes.mcp.enable.hint') }}</FieldDescription>
        <template v-if="draft.enabled">
          <Field>
            <FieldLabel
              >{{ t('common.field.description.label') }}
              <span class="text-subtle"
                >— {{ t('kubernetes.mcp.descriptionNote.label') }}</span
              ></FieldLabel
            >
            <Textarea
              v-model="draft.description"
              class="font-data"
              rows="2"
              maxlength="1000"
              data-testid="kube-mcp-description"
              @keydown="wrapSelectionOnType"
            />
          </Field>
          <Field>
            <FieldLabel>{{ t('kubernetes.mcp.approval.label') }}</FieldLabel>
            <RadioGroup
              :model-value="draft.approval"
              class="gap-1.5"
              data-testid="kube-mcp-approval"
              @update:model-value="(v) => (draft!.approval = v === 'allow' ? 'allow' : 'prompt')"
            >
              <div class="flex items-center gap-2">
                <RadioGroupItem id="kube-mcp-prompt" value="prompt" data-testid="kube-mcp-prompt" />
                <Label for="kube-mcp-prompt">{{ t('automations.form.mcp.prompt.label') }}</Label>
              </div>
              <div class="flex items-center gap-2">
                <RadioGroupItem id="kube-mcp-allow" value="allow" data-testid="kube-mcp-allow" />
                <Label for="kube-mcp-allow">{{ t('kubernetes.mcp.allow.label') }}</Label>
              </div>
            </RadioGroup>
          </Field>
        </template>
        <FieldError v-if="error" data-testid="kube-mcp-error">{{ error }}</FieldError>
      </DialogBody>
      <DialogFooter>
        <Button variant="dialog" size="kira-lg" data-testid="kube-mcp-cancel" @click="open = false">
          {{ t('common.action.cancel.label') }}
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :pending="save.isPending.value"
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
