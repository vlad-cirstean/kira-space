<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLegend,
  FieldSet,
} from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import { useIntervalFn, useNow } from '@vueuse/core';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { formatDateTime, formatRelative } from '@workbench/behaviour/format';
import { TIMING } from '@workbench/behaviour/timing';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref, useId, watch } from 'vue';
import RelativeTime from '../../components/RelativeTime.vue';
import { useMobileAccessStore } from '../../state/mobileAccess';
import { useMobileTerminalsStore } from '../../state/mobileTerminals';
import { MOBILE_PORT_RANGE } from '../../state/settingsDomain';
import MobileQr from './MobileQr.vue';
import type { SettingsPaneProps } from './types';

// Bypasses draft/Save like ConnectedEditorsPane: enabling, trusting a network, port changes and
// revoke are actions on a running server, not settings leaves.
defineProps<SettingsPaneProps>();

// '' is NaN, not 0, so an emptied field fails the whole-number check.
function parseIntField(raw: string): number {
  const text = raw.trim();
  return text === '' ? Number.NaN : Number(text);
}

const now = useNow({ scheduler: (cb) => useIntervalFn(cb, TIMING.pollSlow) });

const store = useMobileAccessStore();
const phones = useMobileTerminalsStore();
const confirmDialogStore = useConfirmDialogStore();
const switchId = useId();
const agentInputId = useId();
const portId = useId();

const portDraft = ref(String(store.status.port));
watch(
  () => store.status.port,
  (port) => {
    portDraft.value = String(port);
  },
);

const portError = computed(() => {
  const port = parseIntField(portDraft.value);
  if (!Number.isInteger(port)) return t('common.enterWholeNumber.error');
  if (port < MOBILE_PORT_RANGE.min || port > MOBILE_PORT_RANGE.max) {
    return t('space.settings.mobileAccess.port.range.error', {
      min: MOBILE_PORT_RANGE.min,
      max: MOBILE_PORT_RANGE.max,
    });
  }
  return null;
});
const portChanged = computed(() => parseIntField(portDraft.value) !== store.status.port);

const { pending: toggling, run: onToggle } = useAsyncAction((on: boolean) => store.setEnabled(on));
const { pending: togglingAgentInput, run: onToggleAgentInput } = useAsyncAction((on: boolean) =>
  store.setAgentInput(on),
);
const { run: onSetPermissions } = useAsyncAction(
  (id: string, write: boolean, agentInput: boolean) =>
    store.setDevicePermissions(id, { write, agentInput }),
);
const { pending: applying, run: onApplyPort } = useAsyncAction(() =>
  store.setPort(parseIntField(portDraft.value)),
);
const { pending: trusting, run: runTrust } = useAsyncAction(() => store.trustNetwork());
const { run: runForget } = useAsyncAction(() => store.forgetNetwork());

const current = computed(() => store.status.current);
const trusted = computed(() => store.status.trusted);
const canTrust = computed(() => {
  const now = current.value;
  const was = trusted.value;
  if (!now) return false;
  return (
    !was ||
    was.subnet !== now.subnet ||
    was.routerIp !== now.routerIp ||
    was.routerMac !== now.routerMac
  );
});

async function onTrust(): Promise<void> {
  const now = current.value;
  if (!now) return;
  const ok = await confirmDialogStore.confirm({
    title: t('space.settings.mobileAccess.trustConfirm.title'),
    message: t('space.settings.mobileAccess.trustConfirm.body', {
      subnet: now.subnet,
      mac: now.routerMac,
    }),
    confirmLabel: t('space.settings.mobileAccess.network.trustVerb.label'),
    tone: 'danger',
  });
  if (ok) await runTrust();
}

async function onForget(): Promise<void> {
  const ok = await confirmDialogStore.confirm({
    title: t('space.settings.mobileAccess.forgetConfirm.title'),
    message: t('space.settings.mobileAccess.forgetConfirm.body'),
    confirmLabel: t('space.settings.mobileAccess.network.forget.label'),
    tone: 'danger',
  });
  if (ok) await runForget();
}

async function onRevoke(id: string, label: string): Promise<void> {
  const ok = await confirmDialogStore.confirm({
    title: t('space.settings.mobileAccess.revokeConfirm.title'),
    message: t('space.settings.mobileAccess.revokeConfirm.body', { name: label || id }),
    confirmLabel: t('space.settings.mobileAccess.devices.revoke.label'),
    tone: 'danger',
  });
  if (ok) await store.revoke(id);
}

const trustedText = computed(() => {
  const net = trusted.value;
  if (!net) return '';
  const at = store.status.trustedAt;
  const params = {
    subnet: net.subnet,
    router: net.routerIp,
    mac: net.routerMac,
    interface: net.interface,
  };
  return at
    ? t('space.settings.mobileAccess.network.trustedAt.status', {
        ...params,
        date: formatDateTime(at),
      })
    : t('space.settings.mobileAccess.network.trusted.status', params);
});

function lastSeenText(device: { lastSeenAt: number; lastIp?: string }): string {
  const when = formatRelative(device.lastSeenAt);
  return device.lastIp
    ? t('space.settings.mobileAccess.devices.lastSeenFrom.status', { when, address: device.lastIp })
    : t('space.settings.mobileAccess.devices.lastSeen.status', { when });
}

const activeDevices = computed(() =>
  store.devices.filter((d) => !d.revokedAt && d.expiresAt > now.value.getTime()),
);
</script>

<template>
  <div class="contents" v-show="active">
    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="switchId">{{ t('space.settings.mobileAccess.enabled.label') }}</Label>
      </div>
      <Switch
        :id="switchId"
        :model-value="store.status.enabled"
        :disabled="toggling"
        data-testid="mobile-access-enabled"
        @update:model-value="(v) => onToggle(v === true)"
      />
      <FieldDescription>
        {{ t('space.settings.mobileAccess.enabled.description') }}
      </FieldDescription>
      <FieldError v-if="store.status.error" data-testid="mobile-access-error">
        {{ store.status.error }}
      </FieldError>
    </Field>

    <Alert variant="warn" data-testid="mobile-access-plaintext-warning">
      {{ t('space.settings.mobileAccess.plaintext.body') }}
    </Alert>

    <FieldSet class="gap-1" data-testid="mobile-access-network">
      <FieldLegend>{{ t('space.settings.mobileAccess.network.title') }}</FieldLegend>
      <p v-if="trusted" class="m-0 text-kira-sm" data-testid="mobile-access-trusted">
        {{ trustedText }}
      </p>
      <FieldDescription v-else data-testid="mobile-access-trusted">
        {{ t('space.settings.mobileAccess.network.none.empty') }}
      </FieldDescription>
      <FieldDescription v-if="current" data-testid="mobile-access-current">
        {{
          t('space.settings.mobileAccess.network.current.status', {
            address: current.address,
            subnet: current.subnet,
            router: current.routerIp,
            mac: current.routerMac,
          })
        }}
      </FieldDescription>
      <FieldDescription v-else data-testid="mobile-access-current">
        {{ t('space.settings.mobileAccess.network.offNetwork.empty') }}
      </FieldDescription>
      <div class="flex gap-2">
        <Button
          variant="dialog"
          size="kira-lg"
          :pending="trusting"
          :disabled="!canTrust"
          data-testid="mobile-access-trust"
          @click="onTrust"
        >
          {{ t('space.settings.mobileAccess.network.trust.label') }}
        </Button>
        <Button
          v-if="trusted"
          variant="dialog"
          size="kira-lg"
          data-testid="mobile-access-forget"
          @click="onForget"
        >
          {{ t('space.settings.mobileAccess.network.forget.label') }}
        </Button>
      </div>
      <p
        v-if="store.status.enabled && !store.status.running && store.status.stopDetail"
        class="m-0 text-kira-sm"
        data-testid="mobile-access-stopped-reason"
      >
        {{ store.status.stopDetail }}
      </p>
    </FieldSet>

    <FieldSet v-if="store.status.running" class="gap-1" data-testid="mobile-access-step-app">
      <FieldLegend>{{ t('space.settings.mobileAccess.openApp.title') }}</FieldLegend>
      <figure class="m-0 flex flex-col gap-0.5">
        <MobileQr :url="store.status.appUrl" />
        <figcaption class="font-data text-kira-sm break-all select-all">{{
          store.status.appUrl
        }}</figcaption>
      </figure>
    </FieldSet>

    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="agentInputId">{{ t('space.settings.mobileAccess.agentInput.label') }}</Label>
      </div>
      <Switch
        :id="agentInputId"
        :model-value="store.status.agentInput"
        :disabled="togglingAgentInput"
        data-testid="mobile-access-agent-input"
        @update:model-value="(v) => onToggleAgentInput(v === true)"
      />
      <FieldDescription>
        {{ t('space.settings.mobileAccess.agentInput.description') }}
      </FieldDescription>
    </Field>

    <Field>
      <Label :for="portId">{{ t('common.port.label') }}</Label>
      <NumberStepperInput
        :id="portId"
        :min="MOBILE_PORT_RANGE.min"
        :max="MOBILE_PORT_RANGE.max"
        data-testid="mobile-access-port"
        :model-value="portDraft"
        @input="(e: Event) => (portDraft = (e.target as HTMLInputElement).value)"
      />
      <FieldError v-if="portError" data-testid="mobile-access-port-error">{{
        portError
      }}</FieldError>
      <Button
        variant="dialog"
        size="kira-lg"
        class="self-start"
        :pending="applying"
        :disabled="!!portError || !portChanged"
        data-testid="mobile-access-apply-port"
        @click="onApplyPort"
      >
        {{ t('space.settings.mobileAccess.port.apply.label') }}
      </Button>
    </Field>

    <FieldSet class="gap-1">
      <FieldLegend>{{ t('space.settings.mobileAccess.devices.title') }}</FieldLegend>
      <FieldDescription v-if="activeDevices.length === 0" data-testid="mobile-devices-empty">
        {{ t('space.settings.mobileAccess.devices.none.empty') }}
      </FieldDescription>
      <ul v-else class="m-0 flex list-none flex-col gap-1 p-0">
        <li
          v-for="device in activeDevices"
          :key="device.id"
          class="flex items-center justify-between gap-1"
          :data-testid="`mobile-device-row-${device.id}`"
        >
          <span class="flex min-w-0 flex-col">
            <span class="truncate">{{ device.label || device.id }}</span>
            <span class="text-subtle text-kira-sm leading-normal">
              {{ lastSeenText(device) }}
            </span>
            <span
              class="text-subtle text-kira-sm leading-normal"
              :data-testid="`mobile-device-expires-${device.id}`"
            >
              <CatalogText k="space.settings.mobileAccess.devices.expires.status">
                <template #when><RelativeTime :at="device.expiresAt" /></template>
              </CatalogText>
            </span>
            <span
              v-if="phones.countFor(device.id) > 0"
              class="text-kira-sm leading-normal"
              :data-testid="`mobile-device-terminals-${device.id}`"
            >
              {{
                t('space.settings.mobileAccess.devices.terminals.status', {
                  count: phones.countFor(device.id),
                })
              }}
            </span>
          </span>
          <span class="flex shrink-0 items-center gap-2">
            <Label class="flex items-center gap-1 text-kira-sm">
              {{ t('space.ade.branchPanel.changes.label') }}
              <Switch
                :model-value="device.canWrite"
                :data-testid="`mobile-device-write-${device.id}`"
                @update:model-value="
                  (v) => onSetPermissions(device.id, v === true, device.canAgentInput)
                "
              />
            </Label>
            <Label class="flex items-center gap-1 text-kira-sm">
              {{ t('space.settings.mobileAccess.devices.agentInput.label') }}
              <Switch
                :model-value="device.canAgentInput"
                :disabled="!store.status.agentInput"
                :data-testid="`mobile-device-agent-input-${device.id}`"
                @update:model-value="
                  (v) => onSetPermissions(device.id, device.canWrite, v === true)
                "
              />
            </Label>
            <TooltipIconButton
              icon="trash"
              :label="t('space.settings.mobileAccess.devices.revoke.label')"
              tone="error"
              :data-testid="`mobile-device-revoke-${device.id}`"
              @click="onRevoke(device.id, device.label)"
            />
          </span>
        </li>
      </ul>
    </FieldSet>
  </div>
</template>
