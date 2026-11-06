<script setup lang="ts">
import type { McpPermissionMode } from '@shared/domain/connection';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
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
import { Textarea } from '@theme/components/ui/textarea';
import { type MessageKey, t } from '@theme/i18n';
import { wrapSelectionOnType } from '@theme/wrapSelection';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref, watch } from 'vue';
import { useClusters, useSetClusterMcp } from '../queries';
import type { KubeMcpGroup, KubeMcpSettings } from '../wire';

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

const GROUPS: { group: KubeMcpGroup; label: MessageKey; note: MessageKey }[] = [
  {
    group: 'discover',
    label: 'kubernetes.mcp.discover.label',
    note: 'kubernetes.mcp.discoverNote.label',
  },
  { group: 'list', label: 'kubernetes.mcp.list.label', note: 'kubernetes.mcp.listNote.label' },
  {
    group: 'describe',
    label: 'kubernetes.detail.tab.describe.label',
    note: 'kubernetes.mcp.describeNote.label',
  },
  { group: 'logs', label: 'docker.tab.logs.label', note: 'kubernetes.mcp.logsNote.label' },
];

function modeItems(group: KubeMcpGroup) {
  const modes: { value: McpPermissionMode; label: string }[] = [
    { value: 'deny', label: t('common.action.deny.label') },
    { value: 'allow', label: t('studio.project.connectionDialog.mcp.allow.label') },
    { value: 'prompt', label: t('studio.project.connectionDialog.mcp.prompt.label') },
  ];
  return modes.map((m) => ({ ...m, testid: `kube-mcp-${group}-${m.value}` }));
}

function setMode(group: KubeMcpGroup, value: string): void {
  if (draft.value) draft.value[group] = value as McpPermissionMode;
}

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
        <Field v-for="g in GROUPS" :key="g.group">
          <FieldLabel
            >{{ t(g.label) }} <span class="text-subtle">— {{ t(g.note) }}</span></FieldLabel
          >
          <SecondaryTabs
            variant="segmented"
            size="kira-lg"
            :model-value="draft[g.group]"
            :items="modeItems(g.group)"
            :data-testid="`kube-mcp-${g.group}`"
            @update:model-value="(v) => setMode(g.group, String(v))"
          />
        </Field>
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
