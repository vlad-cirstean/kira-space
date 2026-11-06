<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
} from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import { formatCount } from '@workbench/behaviour/format';
import { computed, useId } from 'vue';
import {
  type ApiSettings,
  HTTP_VERSIONS,
  MAX_REDIRECTS_RANGE,
  MAX_RESPONSE_MB_RANGE,
  REQUEST_TIMEOUT_MS_RANGE,
} from '../../state/settingsDomain';
import { parseIntField, type SettingsPaneProps } from './types';

// P103 Part 2 (§5.5): extracted verbatim from workbench/SettingsDialog.vue's own
// `v-else-if="activeSection === 'Api'"` branch — P90 §2.7's own global Api section.
const props = defineProps<SettingsPaneProps>();

function onHttpVersionChange(value: unknown): void {
  props.draft.api.httpVersion = String(value) as ApiSettings['httpVersion'];
}
function onRequestTimeoutMsInput(e: Event): void {
  props.draft.api.requestTimeoutMs = parseIntField((e.target as HTMLInputElement).value);
}
function onMaxResponseMbInput(e: Event): void {
  props.draft.api.maxResponseMb = parseIntField((e.target as HTMLInputElement).value);
}
function onSslVerifyChange(checked: boolean): void {
  props.draft.api.sslVerify = checked;
}
function onFollowRedirectsChange(checked: boolean): void {
  props.draft.api.followRedirects = checked;
}
function onMaxRedirectsInput(e: Event): void {
  props.draft.api.maxRedirects = parseIntField((e.target as HTMLInputElement).value);
}
function onDisableCookieJarChange(checked: boolean): void {
  props.draft.api.disableCookieJar = checked;
}

const requestTimeoutMsError = computed<string | null>(() => {
  const v = props.draft.api.requestTimeoutMs;
  if (!Number.isInteger(v)) return t('common.enterWholeNumber.error');
  if (v < REQUEST_TIMEOUT_MS_RANGE.min || v > REQUEST_TIMEOUT_MS_RANGE.max) {
    return t('studio.settings.range.ms.status', {
      min: REQUEST_TIMEOUT_MS_RANGE.min,
      max: formatCount(REQUEST_TIMEOUT_MS_RANGE.max),
    });
  }
  return null;
});
props.registerFieldError('api.requestTimeoutMs', requestTimeoutMsError);

const maxResponseMbError = computed<string | null>(() => {
  const v = props.draft.api.maxResponseMb;
  if (!Number.isInteger(v)) return t('common.enterWholeNumber.error');
  if (v < MAX_RESPONSE_MB_RANGE.min || v > MAX_RESPONSE_MB_RANGE.max) {
    return t('studio.settings.range.mb.status', {
      min: MAX_RESPONSE_MB_RANGE.min,
      max: MAX_RESPONSE_MB_RANGE.max,
    });
  }
  return null;
});
props.registerFieldError('api.maxResponseMb', maxResponseMbError);

const maxRedirectsError = computed<string | null>(() => {
  const v = props.draft.api.maxRedirects;
  if (!Number.isInteger(v)) return t('common.enterWholeNumber.error');
  if (v < MAX_REDIRECTS_RANGE.min || v > MAX_REDIRECTS_RANGE.max) {
    return t('studio.settings.range.plain.status', {
      min: MAX_REDIRECTS_RANGE.min,
      max: MAX_REDIRECTS_RANGE.max,
    });
  }
  return null;
});
props.registerFieldError('api.maxRedirects', maxRedirectsError);

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
  <div class="contents" v-show="active">
    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="httpVersionId">{{ t('studio.http.settings.version.label') }}</Label>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          data-testid="settings-reset-api-httpVersion"
          :disabled-trigger="isAtDefault('api', 'httpVersion')"
          :disabled="isAtDefault('api', 'httpVersion')"
          @click="resetLeaf('api', 'httpVersion')"
        />
      </div>
      <NativeSelect
        :id="httpVersionId"
        variant="bordered"
        size="kira-lg"
        class="self-start"
        data-testid="settings-api-httpVersion"
        :model-value="draft.api.httpVersion"
        @update:model-value="onHttpVersionChange"
      >
        <option v-for="v in HTTP_VERSIONS" :key="v" :value="v">{{
          t('studio.http.settings.versionOption.option', { version: v })
        }}</option>
      </NativeSelect>
    </Field>

    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="requestTimeoutMsId">{{ t('studio.http.settings.timeout.label') }}</Label>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          data-testid="settings-reset-api-requestTimeoutMs"
          :disabled-trigger="isAtDefault('api', 'requestTimeoutMs')"
          :disabled="isAtDefault('api', 'requestTimeoutMs')"
          @click="resetLeaf('api', 'requestTimeoutMs')"
        />
      </div>
      <NumberStepperInput
        :id="requestTimeoutMsId"
        :min="REQUEST_TIMEOUT_MS_RANGE.min"
        :max="REQUEST_TIMEOUT_MS_RANGE.max"
        :aria-invalid="!!requestTimeoutMsError || undefined"
        data-testid="settings-api-requestTimeoutMs"
        :model-value="String(draft.api.requestTimeoutMs)"
        @input="onRequestTimeoutMsInput"
      />
      <FieldError v-if="requestTimeoutMsError" data-testid="settings-api-requestTimeoutMs-error">
        {{ requestTimeoutMsError }}
      </FieldError>
      <FieldDescription v-else>{{
        t('studio.settings.api.requestTimeout.description')
      }}</FieldDescription>
    </Field>

    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="maxResponseMbId">{{ t('studio.http.settings.maxResponse.label') }}</Label>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          data-testid="settings-reset-api-maxResponseMb"
          :disabled-trigger="isAtDefault('api', 'maxResponseMb')"
          :disabled="isAtDefault('api', 'maxResponseMb')"
          @click="resetLeaf('api', 'maxResponseMb')"
        />
      </div>
      <NumberStepperInput
        :id="maxResponseMbId"
        :min="MAX_RESPONSE_MB_RANGE.min"
        :max="MAX_RESPONSE_MB_RANGE.max"
        :aria-invalid="!!maxResponseMbError || undefined"
        data-testid="settings-api-maxResponseMb"
        :model-value="String(draft.api.maxResponseMb)"
        @input="onMaxResponseMbInput"
      />
      <FieldError v-if="maxResponseMbError" data-testid="settings-api-maxResponseMb-error">
        {{ maxResponseMbError }}
      </FieldError>
      <FieldDescription v-else>{{
        t('studio.settings.api.maxResponse.description')
      }}</FieldDescription>
    </Field>

    <FieldGroup>
      <Field orientation="horizontal">
        <Switch
          :id="sslVerifyId"
          :model-value="draft.api.sslVerify"
          data-testid="settings-api-sslVerify"
          @update:model-value="(v) => onSslVerifyChange(v === true)"
        />
        <Label :for="sslVerifyId">{{ t('studio.http.settings.verifySsl.label') }}</Label>
      </Field>
      <TooltipIconButton
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        class="ml-auto"
        data-testid="settings-reset-api-sslVerify"
        :disabled-trigger="isAtDefault('api', 'sslVerify')"
        :disabled="isAtDefault('api', 'sslVerify')"
        @click="resetLeaf('api', 'sslVerify')"
      />
    </FieldGroup>
    <Alert v-if="!draft.api.sslVerify" variant="warn" data-testid="settings-api-sslVerify-warning">
      <AlertDescription>{{ t('studio.settings.api.sslVerify.warning.error') }}</AlertDescription>
    </Alert>

    <FieldGroup>
      <Field orientation="horizontal">
        <Switch
          :id="followRedirectsId"
          :model-value="draft.api.followRedirects"
          data-testid="settings-api-followRedirects"
          @update:model-value="(v) => onFollowRedirectsChange(v === true)"
        />
        <Label :for="followRedirectsId">{{
          t('studio.http.settings.followRedirects.label')
        }}</Label>
      </Field>
      <TooltipIconButton
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        class="ml-auto"
        data-testid="settings-reset-api-followRedirects"
        :disabled-trigger="isAtDefault('api', 'followRedirects')"
        :disabled="isAtDefault('api', 'followRedirects')"
        @click="resetLeaf('api', 'followRedirects')"
      />
    </FieldGroup>

    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="maxRedirectsId">{{ t('studio.http.settings.maxRedirects.label') }}</Label>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          data-testid="settings-reset-api-maxRedirects"
          :disabled-trigger="isAtDefault('api', 'maxRedirects')"
          :disabled="isAtDefault('api', 'maxRedirects')"
          @click="resetLeaf('api', 'maxRedirects')"
        />
      </div>
      <NumberStepperInput
        :id="maxRedirectsId"
        :min="MAX_REDIRECTS_RANGE.min"
        :max="MAX_REDIRECTS_RANGE.max"
        :disabled="!draft.api.followRedirects"
        :aria-invalid="!!maxRedirectsError || undefined"
        data-testid="settings-api-maxRedirects"
        :model-value="String(draft.api.maxRedirects)"
        @input="onMaxRedirectsInput"
      />
      <FieldError v-if="maxRedirectsError" data-testid="settings-api-maxRedirects-error">
        {{ maxRedirectsError }}
      </FieldError>
      <FieldDescription v-else-if="!draft.api.followRedirects">{{
        t('studio.http.settings.redirectsOff.description')
      }}</FieldDescription>
    </Field>

    <FieldGroup>
      <Field orientation="horizontal" class="items-start">
        <Switch
          :id="disableCookieJarId"
          :model-value="draft.api.disableCookieJar"
          data-testid="settings-api-disableCookieJar"
          @update:model-value="(v) => onDisableCookieJarChange(v === true)"
        />
        <FieldContent>
          <Label :for="disableCookieJarId">{{ t('studio.http.settings.disableJar.label') }}</Label>
          <FieldDescription>{{ t('studio.settings.api.cookieJar.description') }}</FieldDescription>
        </FieldContent>
      </Field>
      <TooltipIconButton
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        class="ml-auto"
        data-testid="settings-reset-api-disableCookieJar"
        :disabled-trigger="isAtDefault('api', 'disableCookieJar')"
        :disabled="isAtDefault('api', 'disableCookieJar')"
        @click="resetLeaf('api', 'disableCookieJar')"
      />
    </FieldGroup>
  </div>
</template>
