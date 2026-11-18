<script setup lang="ts">
import CodeBlock from '@theme/components/CodeBlock.vue';
import InlineState from '@theme/components/InlineState.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';
import AdePortBackupsDialog from '../port/AdePortBackupsDialog.vue';
import AdePortExportDialog from '../port/AdePortExportDialog.vue';
import AdePortImportDialog from '../port/AdePortImportDialog.vue';
import { usePortProviders } from '../port/queries';
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

// Import and Export read and write the provider's own user-level files, so they show only for a
// provider the app can port.
const ports = usePortProviders();
const port = computed(() => ports.data.value?.find((p) => p.id === current.value?.id));
const dialog = ref<'import' | 'export' | 'backups' | null>(null);
const importOpen = computed({
  get: () => dialog.value === 'import',
  set: (v) => (dialog.value = v ? 'import' : null),
});
const exportOpen = computed({
  get: () => dialog.value === 'export',
  set: (v) => (dialog.value = v ? 'export' : null),
});
const backupsOpen = computed({
  get: () => dialog.value === 'backups',
  set: (v) => (dialog.value = v ? 'backups' : null),
});
</script>

<template>
  <div class="flex min-w-0 flex-col gap-3" data-testid="ade-settings-result">
    <SectionHeading flush :label="t('common.field.result.label')" />
    <FieldDescription>{{ t('space.ade.settings.result.hint') }}</FieldDescription>
    <SecondaryTabs
      v-if="providers.length > 1"
      v-model="picked"
      :items="items"
      :aria-label="t('space.ade.settings.result.tabs.aria')"
    />
    <div v-if="current" class="flex min-w-0 flex-col gap-3" data-testid="ade-settings-provider">
      <div v-if="port" class="flex items-center gap-3" data-testid="ade-settings-port">
        <Button
          variant="link"
          size="inline"
          data-testid="ade-port-import"
          @click="dialog = 'import'"
        >
          {{ t('space.ade.port.importButton.label', { agent: port.label }) }}
        </Button>
        <Button
          variant="link"
          size="inline"
          data-testid="ade-port-export"
          @click="dialog = 'export'"
        >
          {{ t('space.ade.port.exportButton.label', { agent: port.label }) }}
        </Button>
        <Button
          variant="link"
          size="inline"
          data-testid="ade-port-backups-open"
          @click="dialog = 'backups'"
        >
          {{ t('space.ade.port.backups.label') }}
        </Button>
      </div>
      <section
        v-for="f in current.files"
        :key="f.name"
        class="flex flex-col gap-1"
        data-testid="ade-settings-file"
      >
        <SectionHeading flush data :label="f.name" />
        <CodeBlock class="max-h-96 overflow-auto">{{ f.text }}</CodeBlock>
      </section>
      <section v-if="current.env.length > 0" class="flex flex-col gap-1">
        <SectionHeading flush :label="t('docker.field.env.label')" />
        <ul class="m-0 flex list-none flex-col gap-0.5 p-0" data-testid="ade-settings-env">
          <li v-for="e in current.env" :key="e.name" class="font-data text-kira-sm">
            {{ e.name }}={{ e.value }}
          </li>
        </ul>
      </section>
      <InlineState
        kind="empty"
        v-if="current.files.length === 0 && current.env.length === 0"
        data-testid="ade-settings-empty"
      >
        {{ t('space.ade.settings.result.empty.hint') }}
      </InlineState>
      <ul
        v-if="current.notes.length > 0"
        class="m-0 flex list-none flex-col gap-0.5 p-0 text-kira-sm"
        data-testid="ade-settings-notes"
      >
        <li v-for="n in current.notes" :key="n">{{ n }}</li>
      </ul>
      <section v-if="current.kira.length > 0" class="flex flex-col gap-1">
        <SectionHeading flush :label="t('space.ade.settings.result.kira.label')" />
        <ul
          class="m-0 flex list-none flex-col gap-0.5 p-0 text-kira-sm"
          data-testid="ade-settings-kira"
        >
          <li v-for="k in current.kira" :key="k">{{ k }}</li>
        </ul>
      </section>
    </div>
    <template v-if="port">
      <AdePortImportDialog v-model:open="importOpen" :provider="port" />
      <AdePortExportDialog v-model:open="exportOpen" :provider="port" />
      <AdePortBackupsDialog v-model:open="backupsOpen" :provider="port" />
    </template>
  </div>
</template>
