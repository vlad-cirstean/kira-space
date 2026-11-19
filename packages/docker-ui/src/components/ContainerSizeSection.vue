<script setup lang="ts">
import KeyValueList from '@theme/components/KeyValueList.vue';
import { t } from '@theme/i18n';
import { formatBytes } from '@workbench/behaviour/format';
import { useContainerSize } from '../queries';
import MeasuredSection from './MeasuredSection.vue';

const props = defineProps<{ id: string }>();

const { query } = useContainerSize(() => props.id);
</script>

<template>
  <MeasuredSection
    :title="t('docker.field.size.label')"
    testid="docker-size"
    :query="query"
    :taken-at="(size) => size.takenAt"
    :measure-tooltip="t('docker.size.measure.tooltip')"
    :not-measured="t('docker.size.notMeasured.body')"
  >
    <template #default="{ data: size }">
      <KeyValueList>
        <dt>{{ t('docker.size.writableLayer.label') }}</dt>
        <dd class="font-data" data-testid="docker-size-rw">{{ formatBytes(size.sizeRw) }}</dd>
        <dt>{{ t('docker.size.total.label') }}</dt>
        <dd class="font-data" data-testid="docker-size-rootfs">
          {{ formatBytes(size.sizeRootFs) }}
        </dd>
      </KeyValueList>
    </template>
  </MeasuredSection>
</template>
