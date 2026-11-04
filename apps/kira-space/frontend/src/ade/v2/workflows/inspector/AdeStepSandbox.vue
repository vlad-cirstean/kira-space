<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { FieldDescription } from '@theme/components/ui/field';
import { Textarea } from '@theme/components/ui/textarea';
import { type MessageKey, t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';
import {
  ACCESS_MODES,
  addPaths,
  formatPaths,
  parsePaths,
  pathsValid,
  TOOLCHAINS,
} from '../../board/stepSandbox';
import { useWorkflows } from '../../queries';
import type { SandboxAccess, StepKind, StepSandbox } from '../../wire';
import AdeInspectorRow from './AdeInspectorRow.vue';

// A step's sandbox, for its agent or script: access level, extra folders, network.
const props = defineProps<{ sandbox: StepSandbox; stepId: string; kind: StepKind }>();
const script = computed(() => props.kind === 'script');
const manual = computed(() => props.kind === 'manual');
const unconfinedByDefault = computed(() => script.value || manual.value);
const emit = defineEmits<{ patch: [sandbox: StepSandbox] }>();
const workflows = useWorkflows();
const support = computed(() => workflows.data.value?.sandbox ?? null);
const set = (p: Partial<StepSandbox>): void => emit('patch', { ...props.sandbox, ...p });
const id = (k: string): string => `ade-wf-sandbox-${k}-${props.stepId}`;

const ACCESS_LABEL: Record<SandboxAccess, MessageKey> = {
  write: 'space.ade.sandbox.write.label',
  read: 'space.ade.sandbox.read.label',
  off: 'common.off.status',
};
const items = computed(() =>
  ACCESS_MODES.map((m) => ({
    value: m,
    label: t(ACCESS_LABEL[m]),
    testid: `ade-wf-sandbox-access-${m}`,
  })),
);

const readText = ref(formatPaths(props.sandbox.read));
const allowText = ref(formatPaths(props.sandbox.allow));
watch(
  () => props.sandbox.read,
  (v) => {
    if (formatPaths(parsePaths(readText.value)) !== formatPaths(v)) readText.value = formatPaths(v);
  },
);
watch(
  () => props.sandbox.allow,
  (v) => {
    if (formatPaths(parsePaths(allowText.value)) !== formatPaths(v))
      allowText.value = formatPaths(v);
  },
);
const onRead = (v: string | number): void => {
  readText.value = String(v);
  set({ read: parsePaths(readText.value) });
};
const onAllow = (v: string | number): void => {
  allowText.value = String(v);
  set({ allow: parsePaths(allowText.value) });
};
const readInvalid = computed(() => !pathsValid(props.sandbox.read));
const allowInvalid = computed(() => !pathsValid(props.sandbox.allow));
const PRESET_LABEL = {
  go: 'space.ade.workflows.sandbox.presetGo.option',
  node: 'space.ade.workflows.sandbox.presetNode.option',
  rust: 'space.ade.workflows.sandbox.presetRust.option',
  python: 'space.ade.workflows.sandbox.presetPython.option',
} as const;
const preset = (paths: readonly string[]): void => {
  set({ allow: addPaths(props.sandbox.allow, paths) });
};
</script>

<template>
  <div class="flex flex-col gap-3" data-testid="ade-wf-step-sandbox">
    <AdeInspectorRow :label="t('space.ade.workflows.sandbox.access.label')">
      <SecondaryTabs
        :model-value="sandbox.access"
        :items="items"
        data-testid="ade-wf-sandbox-access"
        @update:model-value="(v: string) => set({ access: v as SandboxAccess })"
      />
    </AdeInspectorRow>
    <FieldDescription
      v-if="sandbox.access === 'off' && unconfinedByDefault"
      data-testid="ade-wf-sandbox-off"
      >{{
        manual
          ? t('space.ade.workflows.sandbox.offManual.hint')
          : t('space.ade.workflows.sandbox.offScript.hint')
      }}</FieldDescription
    >
    <Alert v-else-if="sandbox.access === 'off'" variant="warn" data-testid="ade-wf-sandbox-off">
      <AlertDescription>{{ t('space.ade.workflows.sandbox.offWarning.body') }}</AlertDescription>
    </Alert>
    <template v-else>
      <AdeInspectorRow stack :label="t('space.ade.workflows.sandbox.read.label')" :for="id('read')">
        <Textarea
          :id="id('read')"
          :model-value="readText"
          :placeholder="t('space.ade.workflows.sandbox.read.example')"
          class="min-h-12 w-full resize-y font-data leading-normal"
          data-testid="ade-wf-sandbox-read"
          @update:model-value="onRead"
        />
        <template #hint>
          {{
            sandbox.access === 'read' && script
              ? t('space.ade.workflows.sandbox.readScript.hint')
              : t('space.ade.workflows.sandbox.paths.hint')
          }}
        </template>
        <template v-if="readInvalid" #error>
          <span data-testid="ade-wf-sandbox-read-error">{{
            t('space.ade.workflows.sandbox.pathInvalid.error')
          }}</span>
        </template>
      </AdeInspectorRow>
      <AdeInspectorRow
        v-if="sandbox.access === 'write'"
        stack
        :label="t('space.ade.workflows.sandbox.allow.label')"
        :for="id('allow')"
      >
        <Textarea
          :id="id('allow')"
          :model-value="allowText"
          :placeholder="t('space.ade.workflows.sandbox.allow.example')"
          class="min-h-12 w-full resize-y font-data leading-normal"
          data-testid="ade-wf-sandbox-allow"
          @update:model-value="onAllow"
        />
        <DropdownMenu :modal="false">
          <DropdownMenuTrigger as-child>
            <Button
              variant="dialog"
              size="kira"
              class="self-start"
              data-testid="ade-wf-sandbox-preset"
              >{{
                t('space.ade.workflows.sandbox.preset.label')
              }}</Button
            >
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuItem
              v-for="c in TOOLCHAINS"
              :key="c.id"
              :data-testid="`ade-wf-sandbox-preset-${c.id}`"
              @select="preset(c.paths)"
              >{{
                t(PRESET_LABEL[c.id])
              }}</DropdownMenuItem
            >
          </DropdownMenuContent>
        </DropdownMenu>
        <template v-if="allowInvalid" #error>
          <span data-testid="ade-wf-sandbox-allow-error">{{
            t('space.ade.workflows.sandbox.pathInvalid.error')
          }}</span>
        </template>
      </AdeInspectorRow>
      <AdeInspectorRow :label="t('space.ade.workflows.sandbox.network.label')" :for="id('network')">
        <Checkbox
          :id="id('network')"
          :model-value="sandbox.network"
          data-testid="ade-wf-sandbox-network"
          @update:model-value="(v) => set({ network: v === true })"
        />
        <template #hint>
          {{
            script
              ? t('space.ade.workflows.sandbox.networkScript.hint')
              : t('space.ade.workflows.sandbox.network.hint')
          }}
        </template>
      </AdeInspectorRow>
    </template>
    <FieldDescription
      v-if="manual && sandbox.access !== 'off'"
      data-testid="ade-wf-sandbox-manual-limits"
      >{{
        t('space.ade.workflows.sandbox.manualLimits.hint')
      }}</FieldDescription
    >
    <Alert v-if="support && !support.enforced" variant="info" data-testid="ade-wf-sandbox-notice">
      <AlertDescription>{{ support.reason }}</AlertDescription>
    </Alert>
  </div>
</template>
