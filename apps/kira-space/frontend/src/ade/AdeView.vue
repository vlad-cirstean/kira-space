<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import ModuleStart from '@workbench/components/ModuleStart.vue';
import { useCodeReposStore } from '../state/coderepos';
import AdeShell from './v2/shell/AdeShell.vue';
import { useAdeBoardUiStore } from './v2/state/adeBoardUi';
import { useAdeReposUiStore } from './v2/state/adeReposUi';

// The `ade` mode's root: the v2 shell, or the front door until a repository is imported. The
// Repositories page is where the first import happens, so it shows without one.
const codeReposStore = useCodeReposStore();
const reposUi = useAdeReposUiStore();
const board = useAdeBoardUiStore();
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="ade-view">
    <ModuleStart
      v-if="codeReposStore.records.length === 0 && board.view !== 'repos'"
      icon="robot"
      :title="t('space.ade.view.empty.title')"
      :description="t('space.ade.view.empty.hint')"
    >
      <Button
        variant="dialog-primary"
        size="kira-lg"
        data-testid="ade-import"
        @click="reposUi.open()"
      >
        <CodiconIcon name="repo" :size="13" />
        {{ t('space.ade.view.import.label') }}
      </Button>
    </ModuleStart>
    <AdeShell v-else />
  </div>
</template>
