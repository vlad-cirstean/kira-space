<script setup lang="ts">
import { PALETTE_COLOR_CHOICES } from '@shared/domain/color';
import {
  AWS_STYLE_KINDS,
  type ConnectionKind,
  DEFAULT_PORT,
  FILE_KINDS,
  MIN_SERVER_VERSION,
} from '@shared/domain/connection';
import type { ConnectionFileModel } from '@shared/domain/connectionConfig';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Button } from '@theme/components/ui/button';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colourLabel } from '@theme/connColor';
import { t } from '@theme/i18n';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import SwatchRadio from '@theme/SwatchRadio.vue';
import SecretInput from '@workbench/components/SecretInput.vue';
import { computed, ref } from 'vue';
import { control } from '../../bridge/control';
import ConnectionEnginePicker from './ConnectionEnginePicker.vue';
import { secretRef } from './form';

// Name, colour and how to reach the database. A password or URI typed here is held in `secrets`
// and sent through the write-only call on Save; the file only names it.
const props = defineProps<{
  errorFor: (pointer: string) => string | undefined;
  /** File stem, for the name of a secret this form creates. */
  stem: string;
  savedPassword: boolean;
  savedUri: boolean;
  storageAvailable: boolean;
}>();
const file = defineModel<ConnectionFileModel>({ required: true });
const secrets = defineModel<{ password: string; uri: string }>('secrets', { required: true });

const mode = computed(() => (file.value.mode === 'uri' ? 'uri' : 'fields'));
const kind = computed(() => file.value.kind as ConnectionKind);
const MODE_ITEMS = computed(
  () =>
    [
      { value: 'fields', label: t('studio.common.fields.label'), testid: 'mode-fields' },
      {
        value: 'uri',
        label: t('studio.project.connectionDialog.mode.uri.label'),
        testid: 'mode-uri',
      },
    ] as const,
);

// SQS/S3 have no host/port: fields mode repurposes `database` for the AWS region and `username`
// for a named profile. A file kind (SQLite) carries the absolute path in `database`.
const isAwsStyle = computed(() => AWS_STYLE_KINDS.has(kind.value));
const isFileStyle = computed(() => FILE_KINDS.has(kind.value));
const minVersionNote = computed(() => MIN_SERVER_VERSION[kind.value]);

const pickingEngine = ref(false);
function pickKind(next: ConnectionKind): void {
  file.value.kind = next;
  const port = DEFAULT_PORT[next];
  if (!file.value.port && port) file.value.port = port;
  pickingEngine.value = false;
}

function setMode(next: 'fields' | 'uri'): void {
  file.value.mode = next;
}

function setPort(value: string): void {
  const n = Number.parseFloat(value);
  file.value.port = Number.isNaN(n) ? 0 : n;
}

// A first typed value names the secret the file will point at.
function typeSecret(which: 'password' | 'uri', value: string): void {
  secrets.value = { ...secrets.value, [which]: value };
  if (value && !file.value[which]) file.value[which] = secretRef(`${props.stem}.${which}`);
}

async function onBrowseDatabaseFile(): Promise<void> {
  const res = await control.filesChooseOpen({
    filters: [
      {
        name: t('studio.project.connectionDialog.sqliteFilter.label'),
        extensions: ['sqlite', 'sqlite3', 'db', 'db3'],
      },
      { name: t('studio.common.allFiles.label'), extensions: ['*'] },
    ],
    title: t('studio.project.connectionDialog.databasePicker.title'),
  });
  if (!res.canceled && res.file) file.value.database = res.file.path;
}
</script>

<template>
  <div class="flex flex-col gap-3 p-3" data-testid="connection-section-general">
    <ConnectionEnginePicker v-if="pickingEngine" :kind="kind" @pick="pickKind" />
    <template v-else>
      <div class="flex gap-2 items-start">
        <Field class="flex-2">
          <FieldLabel>{{ t('common.field.name.label') }}</FieldLabel>
          <Input
            :model-value="file.name"
            :aria-invalid="props.errorFor('/name') ? true : undefined"
            class="font-ui"
            data-testid="connection-name"
            @update:model-value="(v) => (file.name = String(v))"
          />
          <FieldError v-if="props.errorFor('/name')" data-testid="connection-name-error">{{
            props.errorFor('/name')
          }}</FieldError>
        </Field>
        <Field class="flex-none">
          <FieldLabel>{{ t('common.field.colour.label') }}</FieldLabel>
          <fieldset
            class="color-picker m-0 flex h-6.5 flex-wrap items-center gap-1 border-0 p-0"
            :aria-label="t('studio.project.connectionDialog.colour.aria')"
          >
            <Tooltip v-for="color in PALETTE_COLOR_CHOICES" :key="color">
              <TooltipTrigger as-child>
                <SwatchRadio
                  name="connection-color"
                  :value="color"
                  :color="color"
                  :checked="(file.color ?? 'none') === color"
                  @change="file.color = color"
                />
              </TooltipTrigger>
              <TooltipContent>{{ colourLabel(color) }}</TooltipContent>
            </Tooltip>
          </fieldset>
        </Field>
      </div>

      <Button
        variant="toolbar"
        size="kira"
        class="self-start"
        data-testid="connection-change-engine"
        @click="pickingEngine = true"
      >
        {{ t('studio.project.connectionDialog.changeEngine.title') }}
      </Button>

      <FieldError v-if="props.errorFor('/kind')" data-testid="connection-kind-error">{{
        props.errorFor('/kind')
      }}</FieldError>
      <FieldDescription v-if="minVersionNote" data-testid="connection-min-version">{{
        minVersionNote
      }}</FieldDescription>

      <Field class="flex-1">
        <FieldLabel>{{ t('common.mode.label') }}</FieldLabel>
        <SecondaryTabs
          variant="segmented"
          size="kira-lg"
          :model-value="mode"
          :items="MODE_ITEMS"
          @update:model-value="(v) => setMode(v as 'fields' | 'uri')"
        />
      </Field>

      <template v-if="mode === 'fields' && isFileStyle">
        <Field class="flex-1">
          <FieldLabel>{{ t('studio.project.connectionDialog.databaseFile.label') }}</FieldLabel>
          <div class="flex items-center gap-1">
            <div class="flex-1 min-w-0">
              <Input
                :model-value="file.database ?? ''"
                :aria-invalid="props.errorFor('/database') ? true : undefined"
                class="font-data"
                data-testid="connection-database"
                @update:model-value="(v) => (file.database = String(v))"
              />
            </div>
            <Button
              variant="dialog"
              size="kira-lg"
              data-testid="connection-browse"
              @click="onBrowseDatabaseFile"
            >
              {{ t('studio.common.browse.label') }}
            </Button>
          </div>
          <FieldError v-if="props.errorFor('/database')">{{
            props.errorFor('/database')
          }}</FieldError>
        </Field>
      </template>
      <template v-else-if="mode === 'fields'">
        <div v-if="!isAwsStyle" class="flex gap-2 items-start">
          <Field class="flex-1">
            <FieldLabel>{{ t('studio.project.connectionDialog.host.label') }}</FieldLabel>
            <Input
              :model-value="file.host ?? ''"
              :aria-invalid="props.errorFor('/host') ? true : undefined"
              class="font-data"
              data-testid="connection-host"
              @update:model-value="(v) => (file.host = String(v))"
            />
            <FieldError v-if="props.errorFor('/host')">{{ props.errorFor('/host') }}</FieldError>
          </Field>
          <Field class="flex-none basis-24">
            <FieldLabel>{{ t('common.port.label') }}</FieldLabel>
            <NumberStepperInput
              :model-value="file.port ? String(file.port) : ''"
              :aria-invalid="props.errorFor('/port') ? true : undefined"
              data-testid="connection-port"
              @update:model-value="(v: string | number) => setPort(String(v))"
            />
            <FieldError v-if="props.errorFor('/port')">{{ props.errorFor('/port') }}</FieldError>
          </Field>
        </div>
        <div v-if="isAwsStyle" class="flex gap-2 items-start">
          <Field class="flex-1">
            <FieldLabel>{{ t('studio.project.connectionDialog.region.label') }}</FieldLabel>
            <Input
              :model-value="file.database ?? ''"
              :aria-invalid="props.errorFor('/database') ? true : undefined"
              class="font-data"
              data-testid="connection-database"
              @update:model-value="(v) => (file.database = String(v))"
            />
            <FieldError v-if="props.errorFor('/database')">{{
              props.errorFor('/database')
            }}</FieldError>
          </Field>
          <Field class="flex-1">
            <FieldLabel>{{ t('studio.project.connectionDialog.awsProfile.label') }}</FieldLabel>
            <Input
              :model-value="file.username ?? ''"
              class="font-data"
              data-testid="connection-username"
              @update:model-value="(v) => (file.username = String(v))"
            />
          </Field>
        </div>
        <template v-else>
          <Field class="flex-1">
            <FieldLabel>{{
              kind === 'kibana'
                ? t('studio.kibana.space.label')
                : t('studio.project.connectionDialog.database.label')
            }}</FieldLabel>
            <Input
              :model-value="file.database ?? ''"
              :placeholder="kind === 'kibana' ? t('common.state.default.option') : undefined"
              :aria-invalid="props.errorFor('/database') ? true : undefined"
              class="font-data"
              data-testid="connection-database"
              @update:model-value="(v) => (file.database = String(v))"
            />
            <FieldError v-if="props.errorFor('/database')">{{
              props.errorFor('/database')
            }}</FieldError>
          </Field>
          <div class="flex gap-2 items-start">
            <Field class="flex-1">
              <FieldLabel>{{ t('common.user.label') }}</FieldLabel>
              <Input
                :model-value="file.username ?? ''"
                class="font-data"
                data-testid="connection-username"
                @update:model-value="(v) => (file.username = String(v))"
              />
            </Field>
            <Field class="flex-1">
              <FieldLabel>{{ t('studio.project.connectionDialog.password.label') }}</FieldLabel>
              <SecretInput
                :model-value="secrets.password"
                :set="savedPassword"
                :disabled="!storageAvailable"
                data-testid="connection-password"
                @update:model-value="(v) => typeSecret('password', v)"
              />
            </Field>
          </div>
        </template>
      </template>
      <Field v-else class="flex-1">
        <FieldLabel>{{ t('studio.project.connectionDialog.mode.uri.label') }}</FieldLabel>
        <SecretInput
          :model-value="secrets.uri"
          :set="savedUri"
          :disabled="!storageAvailable"
          data-testid="connection-uri"
          @update:model-value="(v) => typeSecret('uri', v)"
        />
        <FieldError v-if="props.errorFor('/uri')">{{ props.errorFor('/uri') }}</FieldError>
      </Field>
    </template>
  </div>
</template>
