<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Field, FieldDescription, FieldGroup } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { formatBytes, formatDuration } from '@workbench/behaviour/format';
import { computed, useId } from 'vue';
import { patchHttpRequestTabState } from '../../api/tabs';
import { useSettingsStore } from '../../state/settings';
import {
  HTTP_VERSIONS,
  MAX_REDIRECTS_RANGE,
  MAX_RESPONSE_MB_RANGE,
  REQUEST_TIMEOUT_MS_RANGE,
} from '../../state/settingsDomain';
import type { HttpRequestTabRecord } from '../../state/tabDomain';

// One row per api settings leaf, in SPEC's own order — a control bound to
// tab.state.settings.<leaf>, an "Inherit" checkbox (checked === the leaf is null), and helper text
// naming the inherited value straight off settingsStore.api. No resolver function shared with Go —
// the rule is one `??` per leaf, and writing it twice as a function would be the drift risk, not
// the fix.
const props = defineProps<{ tab: HttpRequestTabRecord }>();
const settingsStore = useSettingsStore();

function patch(fields: Partial<HttpRequestTabRecord['state']['settings']>): void {
  patchHttpRequestTabState(props.tab.id, { settings: { ...props.tab.state.settings, ...fields } });
}

// The schema persists `int().min(0)`: a fractional or negative value would fail hydrate (resetting
// the whole request) and Go's JSON decode. Empty/NaN keeps the previous value.
function boundedInt(
  e: Event,
  range: { readonly min: number; readonly max: number },
): number | undefined {
  const input = e.target as HTMLInputElement;
  const raw = Number(input.value);
  if (input.value.trim() === '' || !Number.isFinite(raw)) return undefined;
  const value = Math.min(range.max, Math.max(range.min, Math.trunc(raw)));
  input.value = String(value);
  return value;
}

const settings = computed(() => props.tab.state.settings);
const global = computed(() => settingsStore.api);

function onHttpVersionChange(value: unknown): void {
  patch({ httpVersion: String(value) as (typeof HTTP_VERSIONS)[number] });
}
function onHttpVersionInherit(inherit: boolean): void {
  patch({ httpVersion: inherit ? null : global.value.httpVersion });
}

function onRequestTimeoutMsInput(e: Event): void {
  const value = boundedInt(e, REQUEST_TIMEOUT_MS_RANGE);
  if (value !== undefined) patch({ requestTimeoutMs: value });
}
function onRequestTimeoutMsInherit(inherit: boolean): void {
  patch({ requestTimeoutMs: inherit ? null : global.value.requestTimeoutMs });
}

function onMaxResponseMbInput(e: Event): void {
  const value = boundedInt(e, MAX_RESPONSE_MB_RANGE);
  if (value !== undefined) patch({ maxResponseMb: value });
}
function onMaxResponseMbInherit(inherit: boolean): void {
  patch({ maxResponseMb: inherit ? null : global.value.maxResponseMb });
}

function onSslVerifyChange(checked: boolean): void {
  patch({ sslVerify: checked });
}
function onSslVerifyInherit(inherit: boolean): void {
  patch({ sslVerify: inherit ? null : global.value.sslVerify });
}

function onFollowRedirectsChange(checked: boolean): void {
  patch({ followRedirects: checked });
}
function onFollowRedirectsInherit(inherit: boolean): void {
  patch({ followRedirects: inherit ? null : global.value.followRedirects });
}

function onMaxRedirectsInput(e: Event): void {
  const value = boundedInt(e, MAX_REDIRECTS_RANGE);
  if (value !== undefined) patch({ maxRedirects: value });
}
function onMaxRedirectsInherit(inherit: boolean): void {
  patch({ maxRedirects: inherit ? null : global.value.maxRedirects });
}

function onDisableCookieJarChange(checked: boolean): void {
  patch({ disableCookieJar: checked });
}
function onDisableCookieJarInherit(inherit: boolean): void {
  patch({ disableCookieJar: inherit ? null : global.value.disableCookieJar });
}

const effectiveFollowRedirects = computed(
  () => settings.value.followRedirects ?? global.value.followRedirects,
);

function onEditGlobalDefaults(): void {
  settingsStore.openSettingsAt('Api');
}

// P110 B40: `.inherit`'s own shared class string, repeated 7 times in this file's template --
// the DEF_TH/DEF_TD const idiom views/definition/*.vue already uses.
const INHERIT_LABEL = 'flex items-center gap-1 text-muted-foreground text-kira-sm';

// P110 I2-26: `for`/`id` preserves the old label-wraps-control implicit association (see
// FontSizeField.vue's own precedent comment) now that the field wrapper is a plain <Field> div.
const httpVersionId = useId();
const requestTimeoutMsId = useId();
const maxResponseMbId = useId();
const sslVerifyId = useId();
const followRedirectsId = useId();
const maxRedirectsId = useId();
const disableCookieJarId = useId();
</script>

<template>
  <div
    class="flex flex-1 min-h-0 flex-col gap-1.5 overflow-auto p-1.5"
    data-testid="http-settings-pane"
  >
    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="httpVersionId">{{ t('studio.http.settings.version.label') }}</Label>
        <Label :class="INHERIT_LABEL">
          <Checkbox
            :model-value="settings.httpVersion === null"
            data-testid="http-settings-httpVersion-inherit"
            @update:model-value="(v) => onHttpVersionInherit(v === true)"
          >
            <CodiconIcon name="check" :size="12" />
          </Checkbox>
          {{ t('common.state.inherit.option') }}
        </Label>
      </div>
      <NativeSelect
        :id="httpVersionId"
        variant="bordered"
        size="kira-lg"
        class="self-start"
        data-testid="http-settings-httpVersion"
        :disabled="settings.httpVersion === null"
        :model-value="settings.httpVersion ?? global.httpVersion"
        @update:model-value="onHttpVersionChange"
      >
        <option v-for="v in HTTP_VERSIONS" :key="v" :value="v">{{
          t('studio.http.settings.versionOption.option', { version: v })
        }}</option>
      </NativeSelect>
      <FieldDescription>{{
        t('studio.http.settings.global.status', {
          value: t('studio.http.settings.versionOption.option', { version: global.httpVersion }),
        })
      }}</FieldDescription>
    </Field>

    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="requestTimeoutMsId">{{ t('studio.http.settings.timeout.label') }}</Label>
        <Label :class="INHERIT_LABEL">
          <Checkbox
            :model-value="settings.requestTimeoutMs === null"
            data-testid="http-settings-requestTimeoutMs-inherit"
            @update:model-value="(v) => onRequestTimeoutMsInherit(v === true)"
          >
            <CodiconIcon name="check" :size="12" />
          </Checkbox>
          {{ t('common.state.inherit.option') }}
        </Label>
      </div>
      <Input
        :id="requestTimeoutMsId"
        type="number"
        size="kira"
        :min="REQUEST_TIMEOUT_MS_RANGE.min"
        :max="REQUEST_TIMEOUT_MS_RANGE.max"
        :disabled="settings.requestTimeoutMs === null"
        data-testid="http-settings-requestTimeoutMs"
        :model-value="String(settings.requestTimeoutMs ?? global.requestTimeoutMs)"
        @input="onRequestTimeoutMsInput"
      />
      <FieldDescription>
        {{
          global.requestTimeoutMs === 0
            ? t('studio.http.settings.globalNoTimeout.status')
            : t('studio.http.settings.global.status', {
                value: formatDuration(global.requestTimeoutMs),
              })
        }}
      </FieldDescription>
    </Field>

    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="maxResponseMbId">{{ t('studio.http.settings.maxResponse.label') }}</Label>
        <Label :class="INHERIT_LABEL">
          <Checkbox
            :model-value="settings.maxResponseMb === null"
            data-testid="http-settings-maxResponseMb-inherit"
            @update:model-value="(v) => onMaxResponseMbInherit(v === true)"
          >
            <CodiconIcon name="check" :size="12" />
          </Checkbox>
          {{ t('common.state.inherit.option') }}
        </Label>
      </div>
      <Input
        :id="maxResponseMbId"
        type="number"
        size="kira"
        :min="MAX_RESPONSE_MB_RANGE.min"
        :max="MAX_RESPONSE_MB_RANGE.max"
        :disabled="settings.maxResponseMb === null"
        data-testid="http-settings-maxResponseMb"
        :model-value="String(settings.maxResponseMb ?? global.maxResponseMb)"
        @input="onMaxResponseMbInput"
      />
      <FieldDescription>
        {{
          global.maxResponseMb === 0
            ? t('studio.http.settings.globalUnlimited.status')
            : t('studio.http.settings.global.status', {
                value: formatBytes(global.maxResponseMb * 1024 * 1024),
              })
        }}
      </FieldDescription>
    </Field>

    <FieldGroup class="items-center">
      <Field orientation="horizontal">
        <Checkbox
          :id="sslVerifyId"
          :model-value="settings.sslVerify ?? global.sslVerify"
          :disabled="settings.sslVerify === null"
          data-testid="http-settings-sslVerify"
          @update:model-value="(v) => onSslVerifyChange(v === true)"
        >
          <CodiconIcon name="check" :size="12" />
        </Checkbox>
        <Label :for="sslVerifyId">{{ t('studio.http.settings.verifySsl.label') }}</Label>
      </Field>
      <Label :class="INHERIT_LABEL">
        <Checkbox
          :model-value="settings.sslVerify === null"
          data-testid="http-settings-sslVerify-inherit"
          @update:model-value="(v) => onSslVerifyInherit(v === true)"
        >
          <CodiconIcon name="check" :size="12" />
        </Checkbox>
        {{ t('common.state.inherit.option') }}
      </Label>
    </FieldGroup>
    <FieldDescription>{{
      t(
        global.sslVerify
          ? 'studio.http.settings.globalOn.status'
          : 'studio.http.settings.globalOff.status',
      )
    }}</FieldDescription>

    <FieldGroup class="items-center">
      <Field orientation="horizontal">
        <Checkbox
          :id="followRedirectsId"
          :model-value="settings.followRedirects ?? global.followRedirects"
          :disabled="settings.followRedirects === null"
          data-testid="http-settings-followRedirects"
          @update:model-value="(v) => onFollowRedirectsChange(v === true)"
        >
          <CodiconIcon name="check" :size="12" />
        </Checkbox>
        <Label :for="followRedirectsId">{{
          t('studio.http.settings.followRedirects.label')
        }}</Label>
      </Field>
      <Label :class="INHERIT_LABEL">
        <Checkbox
          :model-value="settings.followRedirects === null"
          data-testid="http-settings-followRedirects-inherit"
          @update:model-value="(v) => onFollowRedirectsInherit(v === true)"
        >
          <CodiconIcon name="check" :size="12" />
        </Checkbox>
        {{ t('common.state.inherit.option') }}
      </Label>
    </FieldGroup>
    <FieldDescription>{{
      t(
        global.followRedirects
          ? 'studio.http.settings.globalOn.status'
          : 'studio.http.settings.globalOff.status',
      )
    }}</FieldDescription>

    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="maxRedirectsId">{{ t('studio.http.settings.maxRedirects.label') }}</Label>
        <Label :class="INHERIT_LABEL">
          <Checkbox
            :model-value="settings.maxRedirects === null"
            data-testid="http-settings-maxRedirects-inherit"
            @update:model-value="(v) => onMaxRedirectsInherit(v === true)"
          >
            <CodiconIcon name="check" :size="12" />
          </Checkbox>
          {{ t('common.state.inherit.option') }}
        </Label>
      </div>
      <Input
        :id="maxRedirectsId"
        type="number"
        size="kira"
        :min="MAX_REDIRECTS_RANGE.min"
        :max="MAX_REDIRECTS_RANGE.max"
        :disabled="settings.maxRedirects === null || !effectiveFollowRedirects"
        data-testid="http-settings-maxRedirects"
        :model-value="String(settings.maxRedirects ?? global.maxRedirects)"
        @input="onMaxRedirectsInput"
      />
      <FieldDescription v-if="!effectiveFollowRedirects">
        {{ t('studio.http.settings.redirectsOff.description') }}
      </FieldDescription>
      <FieldDescription v-else>{{
        t('studio.http.settings.global.status', { value: global.maxRedirects })
      }}</FieldDescription>
    </Field>

    <FieldGroup class="items-center">
      <Field orientation="horizontal">
        <Checkbox
          :id="disableCookieJarId"
          :model-value="settings.disableCookieJar ?? global.disableCookieJar"
          :disabled="settings.disableCookieJar === null"
          data-testid="http-settings-disableCookieJar"
          @update:model-value="(v) => onDisableCookieJarChange(v === true)"
        >
          <CodiconIcon name="check" :size="12" />
        </Checkbox>
        <Label :for="disableCookieJarId">{{ t('studio.http.settings.disableJar.label') }}</Label>
      </Field>
      <Label :class="INHERIT_LABEL">
        <Checkbox
          :model-value="settings.disableCookieJar === null"
          data-testid="http-settings-disableCookieJar-inherit"
          @update:model-value="(v) => onDisableCookieJarInherit(v === true)"
        >
          <CodiconIcon name="check" :size="12" />
        </Checkbox>
        {{ t('common.state.inherit.option') }}
      </Label>
    </FieldGroup>
    <FieldDescription>{{
      t(
        global.disableCookieJar
          ? 'studio.http.settings.globalOff.status'
          : 'studio.http.settings.globalOn.status',
      )
    }}</FieldDescription>

    <Button
      variant="link"
      size="inline"
      class="self-start"
      data-testid="http-settings-edit-global"
      @click="onEditGlobalDefaults"
    >
      {{ t('studio.http.settings.editGlobal.label') }}
    </Button>
  </div>
</template>
