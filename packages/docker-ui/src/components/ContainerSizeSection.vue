<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import DetailSection from '@theme/components/DetailSection.vue';
import InlineState from '@theme/components/InlineState.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import Spinner from '@theme/components/Spinner.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { formatBytes } from '@workbench/behaviour/format';
import RelativeTime from '@workbench/components/RelativeTime.vue';
import { computed } from 'vue';
import { useContainerSize } from '../queries';

const props = defineProps<{ id: string }>();

const { query, refresh } = useContainerSize(() => props.id);

const size = computed(() => query.data.value);
const measuring = computed(() => query.isFetching.value);
</script>

<template>
  <DetailSection :title="t('docker.field.size.label')">
    <template #actions>
      <span class="ml-auto flex items-center gap-1">
        <span v-if="size" data-testid="docker-size-taken"><RelativeTime :at="size.takenAt" /></span>
        <TooltipIconButton
          icon="refresh"
          :label="t('docker.size.measure.tooltip')"
          :disabled="measuring"
          data-testid="docker-size-refresh"
          @click="refresh"
        />
      </span>
    </template>

    <InlineState v-if="query.error.value" kind="error" data-testid="docker-size-error">{{
      query.error.value.message
    }}</InlineState>

    <div v-if="measuring && !size" class="flex items-center gap-2 text-muted-foreground">
      <Spinner label="" />{{ t('docker.measuring.loading') }}
    </div>
    <div v-else-if="!size" class="flex flex-wrap items-center gap-3 text-muted-foreground">
      <span>{{ t('docker.size.notMeasured.body') }}</span>
      <Button size="kira" variant="toolbar" data-testid="docker-size-measure" @click="refresh"
        ><CodiconIcon name="dashboard" :size="13" />{{ t('docker.measure.label') }}</Button
      >
    </div>
    <KeyValueList v-else>
      <dt>{{ t('docker.size.writableLayer.label') }}</dt>
      <dd class="font-data" data-testid="docker-size-rw">{{ formatBytes(size.sizeRw) }}</dd>
      <dt>{{ t('docker.size.total.label') }}</dt>
      <dd class="font-data" data-testid="docker-size-rootfs">{{ formatBytes(size.sizeRootFs) }}</dd>
    </KeyValueList>
  </DetailSection>
</template>
