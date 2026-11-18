<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { computed, provide, ref } from 'vue';
import { resourceMode, toNumber } from '../lib/editDiff';
import { inPlaceSectionKey } from '../lib/editSection';
import type { EditDraft } from '../state/dockerEdit';
import type { DockerResources, RestartPolicyName } from '../wire';
import EditField from './EditField.vue';
import EditSize from './EditSize.vue';

const props = defineProps<{
  draft: EditDraft;
  errors: Record<string, string>;
  disabled: boolean;
  networkLocked: boolean;
  engineCpus?: number;
  allNetworks: string[];
}>();

provide(inPlaceSectionKey, true);

const ip = computed(() => props.draft.inPlace);
const baseIp = computed(() => props.draft.base.inPlace);
const res = computed(() => ip.value.resources);
const baseRes = computed(() => baseIp.value.resources);

const mode = (k: keyof DockerResources) => resourceMode(baseRes.value, res.value, k);
const changed = (k: keyof DockerResources): boolean => baseRes.value[k] !== res.value[k];
const err = (k: keyof DockerResources): string | undefined => props.errors[`resources.${k}`];

const quotaMode = computed(
  () => res.value.cpuQuota !== 0 || res.value.cpuPeriod !== 0 || baseRes.value.cpuQuota !== 0,
);
const cpusText = computed(() => (res.value.nanoCpus > 0 ? String(res.value.nanoCpus / 1e9) : ''));
const cpuChanged = computed(
  () => changed('nanoCpus') || changed('cpuQuota') || changed('cpuPeriod'),
);
const cpuHint = computed(() =>
  props.engineCpus ? t('docker.edit.engineCpus.hint', { count: props.engineCpus }) : '',
);
const cpusetHint = computed(() =>
  props.engineCpus
    ? t('docker.edit.engineCpuset.hint', { count: props.engineCpus, last: props.engineCpus - 1 })
    : '',
);

const RESTARTS: RestartPolicyName[] = ['no', 'always', 'on-failure', 'unless-stopped'];
const restartChanged = computed(
  () =>
    ip.value.restart.name !== baseIp.value.restart.name ||
    ip.value.restart.maxRetries !== baseIp.value.restart.maxRetries,
);

function setCpus(v: string | number): void {
  const n = toNumber(v);
  res.value.nanoCpus = n > 0 ? Math.round(n * 1e9) : 0;
}

function setRestart(v: unknown): void {
  ip.value.restart.name = v as RestartPolicyName;
  if (ip.value.restart.name !== 'on-failure') ip.value.restart.maxRetries = 0;
}

const available = computed(() =>
  props.allNetworks.filter((n) => !ip.value.networks.some((x) => x.name === n)),
);
const toAdd = ref('');

function addNetwork(): void {
  if (!toAdd.value) return;
  ip.value.networks.push({ name: toAdd.value, aliases: [], ipv4: '', ipv6: '' });
  toAdd.value = '';
}

function removeNetwork(name: string): void {
  ip.value.networks = ip.value.networks.filter((n) => n.name !== name);
}

function setAliases(name: string, text: string): void {
  const n = ip.value.networks.find((x) => x.name === name);
  if (n)
    n.aliases = text
      .split(',')
      .map((a) => a.trim())
      .filter(Boolean);
}

const noAliases = (name: string): boolean =>
  name === 'bridge' || name === 'host' || name === 'none';
const netChanged = (name: string): boolean => {
  const b = baseIp.value.networks.find((n) => n.name === name);
  const d = ip.value.networks.find((n) => n.name === name);
  return !b || (d !== undefined && b.aliases.join(',') !== d.aliases.join(','));
};
</script>

<template>
  <div class="flex flex-col gap-3" data-testid="docker-edit-inplace">
    <EditField
      field="name"
      :label="t('common.field.name.label')"
      mode="now"
      :changed="ip.name !== baseIp.name"
      :error="errors.name"
      v-slot="{ invalid, id }"
    >
      <Input
        :id="id"
        v-model="ip.name"
        :disabled="disabled"
        data-testid="docker-edit-name"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="restart"
      :label="t('docker.field.restartPolicy.label')"
      mode="now"
      :changed="restartChanged"
      :error="errors.restart"
      v-slot="{ invalid, id }"
    >
      <div class="flex items-center gap-2">
        <NativeSelect
          :id="id"
          :model-value="ip.restart.name"
          :disabled="disabled"
          data-testid="docker-edit-restart"
          @update:model-value="setRestart"
          v-bind="invalid"
        >
          <option v-for="r in RESTARTS" :key="r" :value="r">{{ r }}</option>
        </NativeSelect>
        <Input
          :aria-label="t('docker.edit.retries.placeholder')"
          v-if="ip.restart.name === 'on-failure'"
          type="number"
          min="0"
          class="w-24"
          :placeholder="t('docker.edit.retries.placeholder')"
          :model-value="ip.restart.maxRetries"
          :disabled="disabled"
          data-testid="docker-edit-restart-retries"
          @update:model-value="ip.restart.maxRetries = toNumber($event)"
        />
      </div>
    </EditField>

    <EditField
      v-if="!quotaMode"
      field="cpus"
      :label="t('docker.edit.cpus.label')"
      :mode="mode('nanoCpus')"
      :changed="cpuChanged"
      :error="err('nanoCpus')"
      :description="cpuHint"
      v-slot="{ invalid, id }"
    >
      <Input
        :id="id"
        type="number"
        min="0"
        step="0.1"
        :placeholder="t('docker.edit.noLimit.placeholder')"
        :model-value="cpusText"
        :disabled="disabled"
        data-testid="docker-edit-cpus"
        @update:model-value="setCpus"
        v-bind="invalid"
      />
    </EditField>
    <template v-else>
      <EditField
        field="cpuQuota"
        :label="t('docker.edit.cpuQuota.label')"
        :mode="mode('cpuQuota')"
        :changed="changed('cpuQuota')"
        :error="err('cpuQuota')"
        v-slot="{ invalid, id }"
      >
        <Input
          :id="id"
          type="number"
          min="0"
          :model-value="res.cpuQuota"
          :disabled="disabled"
          data-testid="docker-edit-cpu-quota"
          @update:model-value="res.cpuQuota = toNumber($event)"
          v-bind="invalid"
        />
      </EditField>
      <EditField
        field="cpuPeriod"
        :label="t('docker.edit.cpuPeriod.label')"
        :mode="mode('cpuPeriod')"
        :changed="changed('cpuPeriod')"
        :error="err('cpuPeriod')"
        v-slot="{ invalid, id }"
      >
        <Input
          :id="id"
          type="number"
          min="0"
          :model-value="res.cpuPeriod"
          :disabled="disabled"
          data-testid="docker-edit-cpu-period"
          @update:model-value="res.cpuPeriod = toNumber($event)"
          v-bind="invalid"
        />
      </EditField>
    </template>

    <EditField
      field="cpuShares"
      :label="t('docker.edit.cpuShares.label')"
      :mode="mode('cpuShares')"
      :changed="changed('cpuShares')"
      :error="err('cpuShares')"
      v-slot="{ invalid, id }"
    >
      <Input
        :id="id"
        type="number"
        min="0"
        :placeholder="t('common.state.default.option')"
        :model-value="res.cpuShares || ''"
        :disabled="disabled"
        data-testid="docker-edit-cpu-shares"
        @update:model-value="res.cpuShares = toNumber($event)"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="cpusetCpus"
      :label="t('docker.edit.cpuset.label')"
      :mode="mode('cpusetCpus')"
      :changed="changed('cpusetCpus')"
      :error="err('cpusetCpus')"
      :description="cpusetHint"
      v-slot="{ invalid, id }"
    >
      <Input
        :id="id"
        v-model="res.cpusetCpus"
        :placeholder="t('docker.edit.cpusetCpus.example')"
        :disabled="disabled"
        data-testid="docker-edit-cpuset-cpus"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="cpusetMems"
      :label="t('docker.edit.cpusetMems.label')"
      :mode="mode('cpusetMems')"
      :changed="changed('cpusetMems')"
      :error="err('cpusetMems')"
      v-slot="{ invalid, id }"
    >
      <Input
        :id="id"
        v-model="res.cpusetMems"
        :placeholder="t('docker.edit.cpusetMems.example')"
        :disabled="disabled"
        data-testid="docker-edit-cpuset-mems"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="memory"
      :label="t('common.module.memory.label')"
      :mode="mode('memory')"
      :changed="changed('memory')"
      :error="err('memory')"
      v-slot="{ invalid, id }"
    >
      <EditSize
        :id="id"
        v-model="res.memory"
        :disabled="disabled"
        testid="docker-edit-memory"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="memoryReservation"
      :label="t('docker.edit.memoryReservation.label')"
      :mode="mode('memoryReservation')"
      :changed="changed('memoryReservation')"
      :error="err('memoryReservation')"
      v-slot="{ invalid, id }"
    >
      <EditSize
        :id="id"
        v-model="res.memoryReservation"
        :disabled="disabled"
        testid="docker-edit-reservation"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="memorySwap"
      :label="t('docker.edit.swap.label')"
      :mode="mode('memorySwap')"
      :changed="changed('memorySwap')"
      :error="err('memorySwap')"
      :description="t('docker.edit.swap.hint')"
      v-slot="{ invalid, id }"
    >
      <EditSize
        :id="id"
        v-model="res.memorySwap"
        allow-unlimited
        :disabled="disabled"
        testid="docker-edit-swap"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="blkioWeight"
      :label="t('docker.edit.blkio.label')"
      :mode="mode('blkioWeight')"
      :changed="changed('blkioWeight')"
      :error="err('blkioWeight')"
      v-slot="{ invalid, id }"
    >
      <Input
        :id="id"
        type="number"
        min="0"
        :placeholder="t('common.state.default.option')"
        :model-value="res.blkioWeight || ''"
        :disabled="disabled"
        data-testid="docker-edit-blkio"
        @update:model-value="res.blkioWeight = toNumber($event)"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="pidsLimit"
      :label="t('docker.edit.pids.label')"
      :mode="mode('pidsLimit')"
      :changed="changed('pidsLimit')"
      :error="err('pidsLimit')"
      v-slot="{ invalid, id }"
    >
      <Input
        :id="id"
        type="number"
        min="0"
        :placeholder="t('docker.edit.unlimited.label')"
        :model-value="res.pidsLimit || ''"
        :disabled="disabled"
        data-testid="docker-edit-pids"
        @update:model-value="res.pidsLimit = toNumber($event)"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="networks"
      multi
      :label="t('docker.field.networks.label')"
      mode="now"
      :changed="
        ip.networks.length !== baseIp.networks.length || ip.networks.some((n) => netChanged(n.name))
      "
      :error="errors.networks"
      :description="
        networkLocked ? t('docker.edit.networkLocked.hint') : t('docker.edit.aliasesChange.hint')
      "
      v-slot="{ invalid }"
    >
      <div class="flex flex-col gap-1.5" data-testid="docker-edit-networks">
        <div
          v-for="n in ip.networks"
          :key="n.name"
          class="flex items-center gap-2"
          data-testid="docker-edit-network"
          :data-name="n.name"
        >
          <span class="w-40 shrink-0 truncate font-data">{{ n.name }}</span>
          <Input
            class="min-w-0 flex-1"
            :aria-label="t('docker.edit.aliases.placeholder')"
            :placeholder="t('docker.edit.aliases.placeholder')"
            :model-value="n.aliases.join(', ')"
            :disabled="disabled || networkLocked || noAliases(n.name)"
            data-testid="docker-edit-network-aliases"
            @change="setAliases(n.name, ($event.target as HTMLInputElement).value)"
            v-bind="invalid"
          />
          <TextTooltip v-if="n.ipv4 || n.ipv6" :text="t('docker.edit.staticAddress.tooltip')"
            ><span class="shrink-0 font-data text-muted-foreground">{{
              n.ipv4 || n.ipv6
            }}</span></TextTooltip
          >
          <TooltipIconButton
            icon="trash"
            :label="t('docker.edit.detach.tooltip')"
            :disabled="disabled || networkLocked"
            data-testid="docker-edit-network-remove"
            @click="removeNetwork(n.name)"
          />
        </div>
        <div class="flex items-center gap-2">
          <NativeSelect
            :aria-label="t('docker.edit.networkToAdd.label')"
            v-model="toAdd"
            class="w-40"
            :disabled="disabled || networkLocked"
            data-testid="docker-edit-network-select"
          >
            <option value="">{{ t('docker.edit.attachNetwork.option') }}</option>
            <option v-for="n in available" :key="n" :value="n">{{ n }}</option>
          </NativeSelect>
          <Button
            size="kira"
            variant="toolbar"
            :disabled="disabled || networkLocked || !toAdd"
            data-testid="docker-edit-network-add"
            @click="addNetwork"
            ><CodiconIcon name="plug" :size="13" />{{ t('docker.edit.attach.label') }}</Button
          >
        </div>
      </div>
    </EditField>
  </div>
</template>
