<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Field, FieldContent, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { useAgentInfo, useAgentList } from '@workbench/agent/agentInfo';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed, useId } from 'vue';
import { control } from '../../bridge/control';
import type { Settings } from '../../state/settingsDomain';
import type { SettingsPaneProps } from './types';

// P188: independent of the title bar's own keep-awake button: either source is enough to hold the
// assertion. Edited through the dialog's draft/Save flow like every other leaf here.
const props = defineProps<SettingsPaneProps>();
const switchId = useId();
const { info } = useAgentInfo();
const { list } = useAgentList();
const providerId = useId();

type NotifyLeaf = Exclude<
  keyof Settings['agent'],
  'provider' | 'hermetic' | 'keepAwakeWithAgents' | 'usageEnabled'
>;

// P238: master first; the rest are disabled while it is off.
const notifyLeaves = computed<{ leaf: NotifyLeaf; testid: string; label: string }[]>(() => [
  {
    leaf: 'notifyEnabled',
    testid: 'settings-agent-notify',
    label: t('space.settings.agent.notify.label'),
  },
  {
    leaf: 'notifyOnFinished',
    testid: 'settings-agent-notify-finished',
    label: t('space.settings.agent.notifyFinished.label'),
  },
  {
    leaf: 'notifyOnNeedsInput',
    testid: 'settings-agent-notify-needs-input',
    label: t('space.settings.agent.notifyNeedsInput.label'),
  },
  {
    leaf: 'notifyOnRunEnded',
    testid: 'settings-agent-notify-run-ended',
    label: t('space.settings.agent.notifyRunEnded.label'),
  },
  {
    leaf: 'notifyIncludeMessage',
    testid: 'settings-agent-notify-include-message',
    label: t('space.settings.agent.notifyIncludeMessage.label'),
  },
]);

// Finished and needs-input notes come from interactive session events.
const shownNotifyLeaves = computed(() =>
  notifyLeaves.value.filter(
    (n) => info.value.hooks || (n.leaf !== 'notifyOnFinished' && n.leaf !== 'notifyOnNeedsInput'),
  ),
);

function setProvider(id: string): void {
  props.draft.agent.provider = id;
}

function setHermetic(on: boolean): void {
  props.draft.agent.hermetic = on;
}

function setNotify(leaf: NotifyLeaf, on: boolean): void {
  props.draft.agent[leaf] = on;
}

function sendTest(): void {
  fireAndForget(control.agentNotifySendTest(), 'agentNotify');
}

function setUsage(on: boolean): void {
  props.draft.agent.usageEnabled = on;
}

function setKeepAwake(on: boolean): void {
  props.draft.agent.keepAwakeWithAgents = on;
}
</script>

<template>
  <div class="contents" v-show="active">
    <Field>
      <div class="flex items-center justify-between gap-1">
        <FieldLabel :for="providerId">{{ t('memory.common.origin.agent.label') }}</FieldLabel>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          data-testid="settings-reset-agent-provider"
          :disabled-trigger="isAtDefault('agent', 'provider')"
          :disabled="isAtDefault('agent', 'provider')"
          @click="resetLeaf('agent', 'provider')"
        />
      </div>
      <NativeSelect
        :id="providerId"
        variant="bordered"
        data-testid="settings-agent-provider"
        :model-value="draft.agent.provider"
        @update:model-value="(v) => setProvider(String(v))"
      >
        <option
          v-for="e in list"
          :key="e.id"
          :value="e.id"
          :disabled="!e.available && e.id !== draft.agent.provider"
        >
          {{
            e.available
              ? e.productLabel
              : t('space.settings.agent.provider.notFound.label', { agent: e.productLabel })
          }}
        </option>
      </NativeSelect>
      <FieldDescription>
        {{ t('space.settings.agent.provider.description') }}
      </FieldDescription>
    </Field>

    <Field orientation="horizontal">
      <Switch
        :id="`${switchId}-hermetic`"
        :model-value="draft.agent.hermetic"
        data-testid="settings-agent-hermetic"
        @update:model-value="(v) => setHermetic(v === true)"
      />
      <FieldContent>
        <FieldLabel :for="`${switchId}-hermetic`">{{
          t('space.settings.agent.hermetic.label')
        }}</FieldLabel>
        <FieldDescription>
          {{ t('space.settings.agent.hermetic.description') }}
        </FieldDescription>
      </FieldContent>
      <TooltipIconButton
        class="ml-auto"
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        data-testid="settings-reset-agent-hermetic"
        :disabled-trigger="isAtDefault('agent', 'hermetic')"
        :disabled="isAtDefault('agent', 'hermetic')"
        @click="resetLeaf('agent', 'hermetic')"
      />
    </Field>

    <Field orientation="horizontal">
      <Switch
        :id="switchId"
        :model-value="draft.agent.keepAwakeWithAgents"
        data-testid="settings-agent-keep-awake"
        @update:model-value="(v) => setKeepAwake(v === true)"
      />
      <FieldContent>
        <FieldLabel :for="switchId">{{ t('space.settings.agent.keepAwake.label') }}</FieldLabel>
        <FieldDescription>
          {{ t('space.settings.agent.keepAwake.description') }}
        </FieldDescription>
      </FieldContent>
      <TooltipIconButton
        class="ml-auto"
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        data-testid="settings-reset-agent-keepAwakeWithAgents"
        :disabled-trigger="isAtDefault('agent', 'keepAwakeWithAgents')"
        :disabled="isAtDefault('agent', 'keepAwakeWithAgents')"
        @click="resetLeaf('agent', 'keepAwakeWithAgents')"
      />
    </Field>

    <Field
      v-for="n in shownNotifyLeaves"
      :key="n.leaf"
      orientation="horizontal"
      :class="n.leaf === 'notifyEnabled' ? '' : 'pl-4'"
    >
      <Switch
        :id="`${switchId}-${n.leaf}`"
        :model-value="draft.agent[n.leaf]"
        :disabled="n.leaf !== 'notifyEnabled' && !draft.agent.notifyEnabled"
        :data-testid="n.testid"
        @update:model-value="(v) => setNotify(n.leaf, v === true)"
      />
      <FieldContent>
        <FieldLabel :for="`${switchId}-${n.leaf}`">{{ n.label }}</FieldLabel>
        <FieldDescription v-if="n.leaf === 'notifyEnabled'">
          {{ t('space.settings.agent.notify.description') }}
        </FieldDescription>
      </FieldContent>
      <TooltipIconButton
        class="ml-auto"
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        :data-testid="`settings-reset-agent-${n.leaf}`"
        :disabled-trigger="isAtDefault('agent', n.leaf)"
        :disabled="isAtDefault('agent', n.leaf)"
        @click="resetLeaf('agent', n.leaf)"
      />
    </Field>
    <Field>
      <!-- conventions-allow N17: sends a test notification, not an approval -->
      <Button
        variant="dialog"
        size="kira-lg"
        class="self-start"
        :disabled="!draft.agent.notifyEnabled"
        data-testid="settings-agent-notify-test"
        @click="sendTest"
      >
        {{ t('common.sendTest.label') }}
      </Button>
    </Field>

    <Field v-if="info.usage" orientation="horizontal">
      <Switch
        :id="`${switchId}-usage`"
        :model-value="draft.agent.usageEnabled"
        data-testid="settings-agent-usage"
        @update:model-value="(v) => setUsage(v === true)"
      />
      <FieldContent>
        <FieldLabel :for="`${switchId}-usage`">{{
          t('space.settings.agent.usage.label')
        }}</FieldLabel>
        <FieldDescription>
          {{ t('space.settings.agent.usage.description') }}
        </FieldDescription>
      </FieldContent>
      <TooltipIconButton
        class="ml-auto"
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        data-testid="settings-reset-agent-usageEnabled"
        :disabled-trigger="isAtDefault('agent', 'usageEnabled')"
        :disabled="isAtDefault('agent', 'usageEnabled')"
        @click="resetLeaf('agent', 'usageEnabled')"
      />
    </Field>
  </div>
</template>
