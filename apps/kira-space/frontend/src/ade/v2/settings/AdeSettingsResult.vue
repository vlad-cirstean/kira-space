<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';
import type { AgentProviderView } from './types';

// What each provider receives, as the launch would write it: references stay as written, so no
// value is ever on this page.
const props = defineProps<{ providers: AgentProviderView[] }>();

const picked = ref('');
const items = computed(() => props.providers.map((p) => ({ value: p.id, label: p.label })));
const current = computed(
  () => props.providers.find((p) => p.id === picked.value) ?? props.providers[0],
);
watch(
  () => props.providers,
  (list) => {
    if (!list.some((p) => p.id === picked.value)) picked.value = list[0]?.id ?? '';
  },
  { immediate: true },
);
</script>

<template>
  <div class="flex min-w-0 flex-col gap-3" data-testid="ade-settings-result">
    <h3 class="m-0 text-kira-lg font-medium">{{ t('space.ade.settings.result.title') }}</h3>
    <FieldDescription>{{ t('space.ade.settings.result.hint') }}</FieldDescription>
    <SecondaryTabs
      v-if="providers.length > 1"
      v-model="picked"
      :items="items"
      :aria-label="t('space.ade.settings.result.tabs.aria')"
    />
    <div v-if="current" class="flex min-w-0 flex-col gap-3" data-testid="ade-settings-provider">
      <section
        v-for="f in current.files"
        :key="f.name"
        class="flex flex-col gap-1"
        data-testid="ade-settings-file"
      >
        <div class="m-0 font-data text-kira-sm text-subtle">{{ f.name }}</div>
        <pre
          class="m-0 max-h-96 overflow-auto rounded-kira-sm bg-field p-2 font-data text-kira-sm"
        >{{ f.text }}</pre>
      </section>
      <section v-if="current.env.length > 0" class="flex flex-col gap-1">
        <div class="m-0 text-kira-sm text-subtle">{{ t('docker.field.env.label') }}</div>
        <ul class="m-0 flex list-none flex-col gap-0.5 p-0" data-testid="ade-settings-env">
          <li v-for="e in current.env" :key="e.name" class="font-data text-kira-sm">
            {{ e.name }}={{ e.value }}
          </li>
        </ul>
      </section>
      <FieldDescription
        v-if="current.files.length === 0 && current.env.length === 0"
        data-testid="ade-settings-empty"
      >
        {{ t('space.ade.settings.result.empty.hint') }}
      </FieldDescription>
      <ul
        v-if="current.notes.length > 0"
        class="m-0 flex list-none flex-col gap-0.5 p-0 text-kira-sm"
        data-testid="ade-settings-notes"
      >
        <li v-for="n in current.notes" :key="n">{{ n }}</li>
      </ul>
      <section v-if="current.kira.length > 0" class="flex flex-col gap-1">
        <div class="m-0 text-kira-sm text-subtle">{{
          t('space.ade.settings.result.kira.label')
        }}</div>
        <ul
          class="m-0 flex list-none flex-col gap-0.5 p-0 text-kira-sm"
          data-testid="ade-settings-kira"
        >
          <li v-for="k in current.kira" :key="k">{{ k }}</li>
        </ul>
      </section>
    </div>
  </div>
</template>
