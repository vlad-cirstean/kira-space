<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed, ref } from 'vue';
import { useProjectOptions } from '../../../repo/state/projectOptions';
import { useAddTaskRepo } from '../queries';
import { useAdeAddUiStore } from '../state/adeAddUi';
import type { CardModel } from './usePlanModel';

// The card's add button: a repository (its branch is named when work starts) or a new branch.
// No Tooltip around the trigger: it would steal the popover anchor.
const props = defineProps<{ card: CardModel }>();

const addUi = useAdeAddUiStore();
const addRepo = useAddTaskRepo();
const open = ref(false);
const error = ref('');
const draftRepoIds = computed(() =>
  props.card.rows.filter((r) => r.branch.name === '').map((r) => r.branch.codeRepoId),
);
const projects = useProjectOptions(draftRepoIds);

async function pickRepo(codeRepoId: string): Promise<void> {
  error.value = '';
  try {
    await addRepo.mutateAsync({ taskId: props.card.task.id, codeRepoId, base: null });
    open.value = false;
  } catch (err) {
    error.value = errorText(err);
  }
}

function addBranch(): void {
  open.value = false;
  addUi.openAttach(props.card.task.id, props.card.title);
}
</script>

<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <Button
        type="button"
        variant="toolbar"
        size="inline"
        class="size-4.5"
        :aria-label="t('space.ade.plan.card.add.aria')"
        data-testid="ade-card-add"
        @click.stop
      >
        <CodiconIcon name="add" :size="13" />
      </Button>
    </PopoverTrigger>
    <PopoverContent
      align="start"
      class="w-80 gap-0 overflow-hidden p-0"
      data-testid="ade-card-add-menu"
      @click.stop
    >
      <ProjectPicker
        inline
        testid="ade-card-add-repo"
        :projects="projects.options.value"
        :placeholder="t('space.ade.taskTab.addRepo.option')"
        :loading="projects.isPending.value"
        :error="projects.error.value"
        @select="(o) => pickRepo(o.value)"
      />
      <div v-if="error" class="m-2">
        <Alert variant="error" data-testid="ade-card-add-error"
          ><AlertDescription>{{ error }}</AlertDescription></Alert
        >
      </div>
      <Button
        variant="toolbar"
        size="kira"
        class="justify-start border-t border-border"
        data-testid="ade-card-add-branch"
        @click="addBranch"
      >
        {{ t('space.ade.taskTab.addBranch.label') }}…
      </Button>
    </PopoverContent>
  </Popover>
</template>
