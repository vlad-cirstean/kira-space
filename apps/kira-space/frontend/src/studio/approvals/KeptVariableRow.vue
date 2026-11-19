<script setup lang="ts">
import type { KeptUse } from '@shared/domain/kept';
import HighlightedText from '@theme/components/HighlightedText.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { t } from '@theme/i18n';
import RelativeTime from '@workbench/components/RelativeTime.vue';
import KeptValueView from '../kept/KeptValueView.vue';
import { useKeptExpiry } from '../kept/keptExpiry';
import { keptTypeLabel } from '../kept/keptFormat';
import { KEPT_MARK_CLASS } from '../kept/keptHighlight';

// One variable of the card's panel: name inside the same mark as in the request, then type, value,
// source, age, expiry and where the request reads it. The countdown is display only; the backend
// decides expiry.
const props = defineProps<{ use: KeptUse; usedIn: string }>();

const expiry = useKeptExpiry(() => props.use.expiresAt);
</script>

<template>
  <KeyValueList columns="fixed" density="compact">
    <dt>{{ t('common.field.name.label') }}</dt>
    <dd class="font-data">
      <HighlightedText :parts="[{ text: use.name, match: true }]" :mark-class="KEPT_MARK_CLASS" />
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
    <dd data-testid="kept-age"><RelativeTime :at="use.createdAt" /></dd>
    <dt>{{ t('studio.kept.expiry.label') }}</dt>
    <dd data-testid="kept-expiry">{{ expiry }}</dd>
    <template v-if="usedIn">
      <dt>{{ t('studio.kept.usedIn.label') }}</dt>
      <dd data-testid="kept-used-in">{{ usedIn }}</dd>
    </template>
  </KeyValueList>
</template>
