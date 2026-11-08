<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import { Button } from '@theme/components/ui/button';
import { FieldDescription, FieldLegend } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { control } from '../../bridge/control';
import { useAppInfo } from './useAppInfo';

const props = defineProps<{ active: boolean }>();

const info = useAppInfo(() => props.active);

function open(url: string): void {
  fireAndForget(control.openExternal(url), 'settings.about');
}
</script>

<template>
  <div class="contents" v-show="active" data-testid="settings-about">
    <FieldLegend>{{ t('common.product.kiraSpace.label') }}</FieldLegend>
    <FieldDescription v-if="info.isPending.value">{{
      t('common.state.loading.loading')
    }}</FieldDescription>
    <FieldDescription v-else-if="info.error.value" role="alert">{{
      errorText(info.error.value)
    }}</FieldDescription>
    <KeyValueList v-else-if="info.data.value">
      <dt>{{ t('common.version.label') }}</dt>
      <dd>
        <span class="tabular-nums" data-testid="settings-about-version">{{
          info.data.value.appVersion
        }}</span>
      </dd>
      <dt>{{ t('space.settings.about.github.label') }}</dt>
      <dd>
        <Button
          variant="link"
          size="inline"
          data-testid="settings-about-github"
          :aria-label="t('space.settings.about.github.aria')"
          @click="open(info.data.value.repoUrl)"
        >
          {{ info.data.value.repoUrl }}
          <CodiconIcon name="link-external" :size="12" aria-hidden="true" />
        </Button>
      </dd>
    </KeyValueList>
  </div>
</template>
