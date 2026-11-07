<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import ModuleStart from '@workbench/components/ModuleStart.vue';
import { useAdeReposUiStore } from '../ade/v2/state/adeReposUi';

const reposUi = useAdeReposUiStore();
</script>

<template>
  <ModuleStart
    icon="source-control"
    :title="t('space.repo.noRepoOpen.empty')"
    data-testid="git-start"
  >
    <Button
      variant="dialog-primary"
      size="kira-lg"
      data-testid="git-start-import"
      @click="reposUi.open()"
    >
      <CodiconIcon name="repo" :size="13" />
      {{ t('space.repo.manage.label') }}
    </Button>
  </ModuleStart>
</template>
