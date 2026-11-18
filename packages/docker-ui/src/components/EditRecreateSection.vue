<script setup lang="ts">
import { Checkbox } from '@theme/components/ui/checkbox';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { deepEqual, toNumber } from '../lib/editDiff';
import { useVolumes } from '../queries';
import type { EditDraft } from '../state/dockerEdit';
import type { DockerRecreateSpec } from '../wire';
import EditField from './EditField.vue';
import EditRows from './EditRows.vue';

const props = defineProps<{
  draft: EditDraft;
  errors: Record<string, string>;
  disabled: boolean;
  preserved: string[];
}>();

const PROTOCOLS = ['tcp', 'udp', 'sctp'] as const;
const MOUNT_TYPES = ['volume', 'bind'] as const;

const rc = computed(() => props.draft.recreate);
const base = computed(() => props.draft.base.recreate);
const volumes = useVolumes();
const volumeNames = computed(() => (volumes.data.value ?? []).map((v) => v.name));

const changed = (k: keyof DockerRecreateSpec): boolean => !deepEqual(base.value[k], rc.value[k]);

const lines = (key: 'cmd' | 'entrypoint') =>
  computed({
    get: () => rc.value[key].join('\n'),
    set: (text: string) => {
      rc.value[key] = text === '' ? [] : text.split('\n');
    },
  });
const cmdText = lines('cmd');
const entrypointText = lines('entrypoint');

const split = (entry: string): [string, string] => {
  const i = entry.indexOf('=');
  return i === -1 ? [entry, ''] : [entry.slice(0, i), entry.slice(i + 1)];
};

function setEnv(index: number, key: string, value: string): void {
  rc.value.env[index] = `${key}=${value}`;
}

const labelEntries = computed(() => Object.entries(rc.value.labels));

function setLabel(index: number, key: string, value: string): void {
  const entries = Object.entries(rc.value.labels);
  entries[index] = [key, value];
  rc.value.labels = Object.fromEntries(entries);
}

function removeLabel(index: number): void {
  rc.value.labels = Object.fromEntries(
    Object.entries(rc.value.labels).filter((_, i) => i !== index),
  );
}

function addLabel(): void {
  if (!('' in rc.value.labels)) rc.value.labels = { ...rc.value.labels, '': '' };
}

function addPort(): void {
  rc.value.ports.push({ containerPort: 80, proto: 'tcp', hostIp: '', hostPort: '' });
}

function addMount(): void {
  rc.value.mounts.push({ key: '', type: 'volume', source: '', target: '/', readOnly: false });
}
</script>

<template>
  <div class="flex flex-col gap-3" data-testid="docker-edit-recreate">
    <EditField
      field="image"
      :label="t('docker.field.image.label')"
      mode="recreate"
      :changed="changed('image')"
      :error="errors['recreate.image']"
      :description="t('docker.edit.imagePresent.hint')"
      v-slot="{ invalid, id }"
    >
      <Input
        :id="id"
        v-model="rc.image"
        :disabled="disabled"
        data-testid="docker-edit-image"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="user"
      :label="t('common.user.label')"
      mode="recreate"
      :changed="changed('user')"
      v-slot="{ invalid, id }"
    >
      <Input
        :id="id"
        v-model="rc.user"
        :placeholder="t('docker.edit.imageDefault.placeholder')"
        :disabled="disabled"
        data-testid="docker-edit-user"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="hostname"
      :label="t('docker.edit.hostname.label')"
      mode="recreate"
      :changed="changed('hostname')"
      :description="t('docker.edit.hostname.hint')"
      v-slot="{ invalid, id }"
    >
      <Input
        :id="id"
        v-model="rc.hostname"
        :disabled="disabled"
        data-testid="docker-edit-hostname"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="entrypoint"
      :label="t('docker.edit.entrypoint.label')"
      mode="recreate"
      :changed="changed('entrypoint')"
      v-slot="{ invalid, id }"
    >
      <Textarea
        class="font-data"
        :id="id"
        v-model="entrypointText"
        :disabled="disabled"
        data-testid="docker-edit-entrypoint"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="cmd"
      :label="t('docker.edit.command.label')"
      mode="recreate"
      :changed="changed('cmd')"
      v-slot="{ invalid, id }"
    >
      <Textarea
        class="font-data"
        :id="id"
        v-model="cmdText"
        :disabled="disabled"
        data-testid="docker-edit-cmd"
        v-bind="invalid"
      />
    </EditField>

    <EditField
      field="env"
      multi
      :label="t('docker.field.env.label')"
      mode="recreate"
      :changed="changed('env')"
      :error="errors['recreate.env']"
      v-slot="{ invalid }"
    >
      <EditRows
        :items="rc.env"
        :add-label="t('docker.edit.addVariable.label')"
        :disabled="disabled"
        testid="docker-edit-env"
        @add="rc.env.push('=')"
        @remove="rc.env.splice($event, 1)"
      >
        <template #row="{ item, index }">
          <Input
            class="w-40"
            :placeholder="t('docker.edit.envKey.example')"
            :model-value="split(item)[0]"
            :disabled="disabled"
            data-testid="docker-edit-env-key"
            @update:model-value="setEnv(index, String($event), split(item)[1])"
            v-bind="invalid"
          />
          <Input
            :aria-label="t('common.field.value.label')"
            class="min-w-0 flex-1"
            :placeholder="t('common.field.value.label')"
            :model-value="split(item)[1]"
            :disabled="disabled"
            data-testid="docker-edit-env-value"
            @update:model-value="setEnv(index, split(item)[0], String($event))"
          />
        </template>
      </EditRows>
    </EditField>

    <EditField
      field="labels"
      multi
      :label="t('docker.field.labels.label')"
      mode="recreate"
      :changed="changed('labels')"
      v-slot="{ invalid }"
    >
      <EditRows
        :items="labelEntries"
        :add-label="t('docker.edit.addLabel.label')"
        :disabled="disabled"
        testid="docker-edit-labels"
        @add="addLabel"
        @remove="removeLabel"
      >
        <template #row="{ item, index }">
          <Input
            class="w-40"
            :placeholder="t('common.field.key.label')"
            :model-value="item[0]"
            :disabled="disabled"
            data-testid="docker-edit-label-key"
            @update:model-value="setLabel(index, String($event), item[1])"
            v-bind="invalid"
          />
          <Input
            :aria-label="t('common.field.value.label')"
            class="min-w-0 flex-1"
            :placeholder="t('common.field.value.label')"
            :model-value="item[1]"
            :disabled="disabled"
            data-testid="docker-edit-label-value"
            @update:model-value="setLabel(index, item[0], String($event))"
          />
        </template>
      </EditRows>
    </EditField>

    <EditField
      field="ports"
      multi
      :label="t('docker.field.ports.label')"
      mode="recreate"
      :changed="changed('ports')"
      :error="errors['recreate.ports']"
      v-slot="{ invalid }"
    >
      <EditRows
        :items="rc.ports"
        :add-label="t('docker.edit.addPort.label')"
        :disabled="disabled"
        testid="docker-edit-ports"
        @add="addPort"
        @remove="rc.ports.splice($event, 1)"
      >
        <template #row="{ item }">
          <Input
            class="w-24"
            type="number"
            :placeholder="t('docker.edit.containerPort.placeholder')"
            :model-value="item.containerPort"
            :disabled="disabled"
            data-testid="docker-edit-port-container"
            @update:model-value="item.containerPort = toNumber($event)"
            v-bind="invalid"
          />
          <NativeSelect
            :aria-label="t('docker.edit.protocol.label')"
            v-model="item.proto"
            class="w-24"
            :disabled="disabled"
            data-testid="docker-edit-port-proto"
          >
            <option v-for="p in PROTOCOLS" :key="p" :value="p">{{ p }}</option>
          </NativeSelect>
          <Input
            :aria-label="t('docker.edit.hostIp.placeholder')"
            class="w-40"
            :placeholder="t('docker.edit.hostIp.placeholder')"
            v-model="item.hostIp"
            :disabled="disabled"
            data-testid="docker-edit-port-ip"
          />
          <Input
            :aria-label="t('docker.edit.hostPort.placeholder')"
            class="w-24"
            :placeholder="t('docker.edit.hostPort.placeholder')"
            v-model="item.hostPort"
            :disabled="disabled"
            data-testid="docker-edit-port-host"
          />
        </template>
      </EditRows>
    </EditField>

    <EditField
      field="mounts"
      multi
      :label="t('docker.field.mounts.label')"
      mode="recreate"
      :changed="changed('mounts')"
      :error="errors['recreate.mounts']"
      v-slot="{ invalid }"
    >
      <datalist id="docker-edit-volumes">
        <option v-for="v in volumeNames" :key="v" :value="v" />
      </datalist>
      <EditRows
        :items="rc.mounts"
        :add-label="t('docker.edit.addMount.label')"
        :disabled="disabled"
        testid="docker-edit-mounts"
        @add="addMount"
        @remove="rc.mounts.splice($event, 1)"
      >
        <template #row="{ item }">
          <NativeSelect
            v-model="item.type"
            class="w-24"
            :disabled="disabled"
            data-testid="docker-edit-mount-type"
            v-bind="invalid"
          >
            <option v-for="m in MOUNT_TYPES" :key="m" :value="m">{{ m }}</option>
          </NativeSelect>
          <Input
            :aria-label="t('docker.edit.mountSource.label')"
            class="min-w-0 flex-1"
            :placeholder="t('docker.edit.mountSource.placeholder')"
            :list="item.type === 'volume' ? 'docker-edit-volumes' : undefined"
            v-model="item.source"
            :disabled="disabled"
            data-testid="docker-edit-mount-source"
          />
          <Input
            :aria-label="t('docker.edit.mountTarget.label')"
            class="min-w-0 flex-1"
            :placeholder="t('docker.edit.mountTarget.example')"
            v-model="item.target"
            :disabled="disabled"
            data-testid="docker-edit-mount-target"
          />
          <Label class="flex shrink-0 items-center gap-1 text-muted-foreground"
            ><Checkbox
              v-model="item.readOnly"
              :disabled="disabled"
              data-testid="docker-edit-mount-ro"
              :aria-label="t('common.state.readOnly.status')"
            />{{ t('docker.edit.readOnlyShort.example') }}</Label
          >
        </template>
      </EditRows>
    </EditField>

    <EditField
      field="capAdd"
      multi
      :label="t('docker.edit.capAdd.label')"
      mode="recreate"
      :changed="changed('capAdd')"
      v-slot="{ invalid }"
    >
      <EditRows
        :items="rc.capAdd"
        :add-label="t('docker.edit.addCap.label')"
        :disabled="disabled"
        testid="docker-edit-cap-add"
        @add="rc.capAdd.push('')"
        @remove="rc.capAdd.splice($event, 1)"
      >
        <template #row="{ index }">
          <Input
            class="min-w-0 flex-1"
            :placeholder="t('docker.edit.capAdd.example')"
            v-model="rc.capAdd[index]"
            :disabled="disabled"
            data-testid="docker-edit-cap-add-input"
            v-bind="invalid"
          />
        </template>
      </EditRows>
    </EditField>

    <EditField
      field="capDrop"
      multi
      :label="t('docker.edit.capDrop.label')"
      mode="recreate"
      :changed="changed('capDrop')"
      v-slot="{ invalid }"
    >
      <EditRows
        :items="rc.capDrop"
        :add-label="t('docker.edit.dropCap.label')"
        :disabled="disabled"
        testid="docker-edit-cap-drop"
        @add="rc.capDrop.push('')"
        @remove="rc.capDrop.splice($event, 1)"
      >
        <template #row="{ index }">
          <Input
            class="min-w-0 flex-1"
            :placeholder="t('docker.edit.capDrop.example')"
            v-model="rc.capDrop[index]"
            :disabled="disabled"
            data-testid="docker-edit-cap-drop-input"
            v-bind="invalid"
          />
        </template>
      </EditRows>
    </EditField>

    <div v-if="preserved.length" class="text-muted-foreground" data-testid="docker-edit-preserved">
      <span class="font-medium">{{ t('docker.edit.preserved.label') }}:</span>
      {{ preserved.join(', ') }}
    </div>
  </div>
</template>
