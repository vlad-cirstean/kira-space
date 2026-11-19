<script setup lang="ts">
import type { KeptUse } from '@shared/domain/kept';
import KeyValueList from '@theme/components/KeyValueList.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { formatCompactDuration } from '@theme/format';
import { t } from '@theme/i18n';
import { useCountdown } from '@workbench/approvals/useCountdown';
import { useRelativeTime } from '@workbench/behaviour/format';
import { computed } from 'vue';
import KeptValueView from '../kept/KeptValueView.vue';
import { keptTypeLabel } from '../kept/keptFormat';
import { KEPT_MARK_CLASS } from '../kept/keptHighlight';

// One variable of the card's panel: name inside the same mark as in the request, then type, value,
// source, age, expiry and where the request reads it. The countdown is display only; the backend
// decides expiry.
const props = defineProps<{ use: KeptUse; usedIn: string }>();

const age = useRelativeTime(() => props.use.createdAt);
const left = useCountdown(() => new Date(props.use.expiresAt).getTime());
const expiry = computed(() =>
  left.value === null || left.value <= 0
    ? t('workbench.approvals.expired.status')
    : t('common.expires.status', {
        time: formatCompactDuration(left.value * 1000, { ceil: true }),
      }),
);
</script>

<template>
  <KeyValueList columns="fixed" density="compact">
    <dt>{{ t('common.field.name.label') }}</dt>
    <dd class="font-data">
      <mark :class="['rounded-kira-sm text-inherit', KEPT_MARK_CLASS]">{{ use.name }}</mark>
      <span class="text-muted-foreground"> · {{ keptTypeLabel(use) }}</span>
    </dd>
    <dt>{{ t('common.field.value.label') }}</dt>
    <dd><KeptValueView :entry="use" /></dd>
    <dt>{{ t('studio.kept.source.label') }}</dt>
    <dd class="font-data">
      <TruncatedText :text="use.source.request" />
      <span v-if="use.source.client" class="text-muted-foreground">{{ use.source.client }}</span>
    </dd>
    <dt>{{ t('studio.kept.keptAt.label') }}</dt>
    <dd data-testid="kept-age">{{ age }}</dd>
    <dt>{{ t('studio.kept.expiry.label') }}</dt>
    <dd data-testid="kept-expiry">{{ expiry }}</dd>
    <template v-if="usedIn">
      <dt>{{ t('studio.kept.usedIn.label') }}</dt>
      <dd data-testid="kept-used-in">{{ usedIn }}</dd>
    </template>
  </KeyValueList>
</template>
