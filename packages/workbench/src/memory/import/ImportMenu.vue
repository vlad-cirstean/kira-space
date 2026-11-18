<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import MenuItems from '../../components/MenuItems.vue';
import type { MenuItem } from '../../state/contextMenu';
import { useChooseImport } from './importQueries';
import { useImportUiStore } from './importStore';

// P211: header menu — pick documents or a folder, scan them, then confirm in ImportConfirmDialog.
const ui = useImportUiStore();
const choose = useChooseImport();

function pick(kind: 'files' | 'folder'): void {
  choose.mutate(kind, {
    onSuccess: (job) => {
      if (job) ui.confirmJobId = job.id;
    },
  });
}

const items = computed<MenuItem[]>(() => [
  {
    type: 'item',
    id: 'files',
    testId: 'memory-import-files',
    label: t('memory.import.files.label'),
    icon: 'files',
    run: () => pick('files'),
  },
  {
    type: 'item',
    id: 'folder',
    testId: 'memory-import-folder',
    label: t('memory.import.folder.label'),
    icon: 'folder',
    run: () => pick('folder'),
  },
]);
</script>

<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <TooltipIconButton
        icon="cloud-upload"
        :label="t('memory.import.documents.label')"
        :disabled="choose.isPending.value"
        data-testid="memory-import"
      />
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end">
      <MenuItems :items="items" />
    </DropdownMenuContent>
  </DropdownMenu>
</template>
