<script setup lang="ts">
import { pathTail } from '@shared/domain/tree';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Button } from '@theme/components/ui/button';
import { colorMarkClass, connTextClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { formatRelative } from '@workbench/behaviour/format';
import Identifier from '@workbench/components/Identifier.vue';
import ModuleStart from '@workbench/components/ModuleStart.vue';
import { computed } from 'vue';
import { useConnectionDialogStore } from '../../state/connectionDialogs';
import { useConnectionsStore } from '../../state/connections';
import { useDatagripImportStore } from '../../state/datagripImport';
import { type RecentTableEntry, useRecentTablesStore, useTabsStore } from '../../state/tabs';

// P1 C4: Studio's two empty states, extracted verbatim out of MainView.vue's old dispatch
// chain (F9) — the mode's own "nothing open" content, mirroring api/ApiStart.vue (C6).

// P16 design system's FirstRun.html: one door, no vestibule. The engine grid lives only in
// the New connection dialog, never repeated at the top level.
const datagripImportStore = useDatagripImportStore();
const importDataGrip = useAsyncAction(() => datagripImportStore.pickAndScanDataGripProject());
const connectionsStore = useConnectionsStore();
const connectionDialogStore = useConnectionDialogStore();
const recentTablesStore = useRecentTablesStore();
const tabsStore = useTabsStore();

const hasConnections = computed(() => connectionsStore.records.length > 0);

function connectionFor(entry: RecentTableEntry) {
  return connectionsStore.connectionRecord(entry.connectionId);
}

// An entry whose connection is gone would open a tab that can never be saved.
const recentEntries = computed(() => recentTablesStore.entries.filter((e) => connectionFor(e)));

function iconFor(entry: RecentTableEntry): string {
  if (entry.kind === 'document') return 'json';
  // P17: a 'keyvalue' entry is a redis key OR an s3 object — same pathTail-kind check
  // TabStrip.vue's own iconFor makes for the live tab icon.
  if (entry.kind === 'keyvalue')
    return pathTail(entry.path)?.kind === 'object' ? 'file' : 'symbol-key';
  if (entry.kind === 'stream') return 'broadcast';
  return 'table';
}

function iconColorClass(entry: RecentTableEntry): string {
  if (entry.kind === 'document') {
    return connTextClass(connectionFor(entry)?.color) ?? 'text-muted-foreground';
  }
  return 'text-info';
}

function openRecent(entry: RecentTableEntry): void {
  if (entry.kind === 'data') tabsStore.openDataTab(entry.connectionId, entry.path);
  else if (entry.kind === 'document') tabsStore.openDocumentTab(entry.connectionId, entry.path);
  else if (entry.kind === 'keyvalue') tabsStore.openKeyValueTab(entry.connectionId, entry.path);
  else tabsStore.openStreamTab(entry.connectionId, entry.path);
}
</script>

<template>
  <!-- FirstRun.html — no connections at all: one button, no duplicate engine grid. -->
  <ModuleStart
    v-if="!hasConnections"
    icon="database"
    :title="t('studio.start.empty.title')"
    :description="t('studio.start.empty.body')"
    data-testid="first-run"
  >
    <Button
      type="button"
      variant="dialog-primary"
      size="kira-lg"
      class="gap-1"
      @click="connectionDialogStore.openNew"
    >
      <span class="size-4 flex items-center justify-center shrink-0"
        ><CodiconIcon :name="ICON.add" :size="13" /></span
      >
      {{ t('studio.project.connectionDialog.newConnection.title') }}
    </Button>
    <template #secondary>
      <Button
        type="button"
        variant="dialog"
        size="kira-lg"
        class="gap-1"
        :pending="importDataGrip.pending.value"
        data-testid="first-run-import-datagrip"
        @click="importDataGrip.run"
      >
        <span class="size-4 flex items-center justify-center shrink-0"
          ><CodiconIcon name="cloud-download" :size="13" /></span
        >
        {{ t('studio.start.importDataGrip.label') }}
      </Button>
    </template>
  </ModuleStart>

  <!-- Empty.html — connections exist, nothing open: recent tables and nothing else. -->
  <ModuleStart
    v-else
    icon="database"
    :title="t('common.product.kiraStudio.label')"
    :description="t('studio.start.open.hint')"
    data-testid="no-tab-open"
  >
    <template v-if="recentEntries.length > 0" #below>
      <SectionHeading :label="t('studio.start.recent.label')" class="mb-1.5 px-0" />
      <div class="flex flex-col">
        <button
          v-for="entry in recentEntries"
          :key="`${entry.kind}:${entry.connectionId}:${entry.path}`"
          type="button"
          :class="cn(rowVariants(), 'w-full text-left')"
          @click="openRecent(entry)"
        >
          <span :class="colorMarkClass('bar', connectionFor(entry)?.color)" />
          <span
            class="size-4 flex items-center justify-center shrink-0"
            :class="iconColorClass(entry)"
          >
            <CodiconIcon :name="iconFor(entry)" :size="13" />
          </span>
          <Identifier :value="entry.path" />
          <span class="ml-auto text-kira-sm text-subtle"
            >{{ connectionFor(entry)?.name }}
            · {{ formatRelative(entry.openedAt) }}</span
          >
        </button>
      </div>
    </template>
  </ModuleStart>
</template>
