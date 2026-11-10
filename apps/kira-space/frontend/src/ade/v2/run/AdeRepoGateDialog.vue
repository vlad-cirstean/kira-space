<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed, ref, watch } from 'vue';
import { useProjectOptions } from '../../../repo/state/projectOptions';
import AdeRepoTag from '../AdeRepoTag.vue';
import { usePlanModel } from '../plan/usePlanModel';
import { useAdeRepoGateStore } from '../state/adeRepoGate';
import { useAdeReposUiStore } from '../state/adeReposUi';

// Choose at least one repo before work starts on a task with no repo: the agent never opens in
// the task folder. Continue adds the picks to the task; the start that asked for them goes on.
const gate = useAdeRepoGateStore();
const reposUi = useAdeReposUiStore();
const { model } = usePlanModel();

const open = computed(() => gate.request !== null);
const card = computed(() =>
  gate.request ? (model.value?.cardFor(gate.request.taskId) ?? null) : null,
);
const chosen = ref<string[]>([]);
const picked = ref<string | null>(null);

const taken = computed(() => [
  ...chosen.value,
  ...(card.value?.rows.map((r) => r.branch.codeRepoId) ?? []),
]);
const projects = useProjectOptions(taken);
const all = useProjectOptions();
const labelOf = (id: string): string => all.options.value.find((o) => o.value === id)?.label ?? id;
const registered = computed(() => all.options.value.length > 0);

watch(open, (o) => {
  if (!o) return;
  chosen.value = [];
  picked.value = null;
});

function pick(id: string): void {
  picked.value = null;
  if (!chosen.value.includes(id)) chosen.value.push(id);
}
function drop(id: string): void {
  chosen.value = chosen.value.filter((x) => x !== id);
}
function openRepos(): void {
  gate.cancel();
  void reposUi.open();
}
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && gate.cancel()">
    <DialogContent size="sm" data-testid="ade-repo-gate">
      <DialogHeader>
        <DialogTitle>{{ t('space.ade.repoGate.title') }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <FieldDescription>{{
          t('space.ade.repoGate.body', { task: card?.title ?? '' })
        }}</FieldDescription>
        <div v-if="registered" class="flex flex-col gap-2">
          <ProjectPicker
            v-if="projects.options.value.length > 0"
            v-model="picked"
            testid="ade-repo-gate-pick"
            :projects="projects.options.value"
            :placeholder="t('space.ade.repoGate.pick.option')"
            :aria-label="t('space.ade.repoGate.pick.aria')"
            @select="(o) => pick(o.value)"
          />
          <div v-if="chosen.length" class="flex flex-wrap gap-1.5">
            <span
              v-for="id in chosen"
              :key="id"
              class="inline-flex items-center gap-0.5"
              data-testid="ade-repo-gate-chip"
            >
              <AdeRepoTag :code-repo-id="id" :label="labelOf(id)" />
              <Button
                variant="toolbar"
                size="kira-icon"
                :aria-label="t('space.ade.repoGate.remove.aria', { repo: labelOf(id) })"
                @click="drop(id)"
              >
                <CodiconIcon name="close" :size="12" />
              </Button>
            </span>
          </div>
        </div>
        <div v-else class="flex flex-col items-start gap-2" data-testid="ade-repo-gate-no-repos">
          <FieldDescription>{{ t('space.ade.repoGate.noRepos.hint') }}</FieldDescription>
          <Button variant="dialog" size="kira-lg" @click="openRepos">{{
            t('space.ade.repoGate.noRepos.action.label')
          }}</Button>
        </div>
        <Alert v-if="gate.error" variant="error" data-testid="ade-repo-gate-error">
          <AlertDescription>{{ gate.error }}</AlertDescription>
        </Alert>
      </DialogBody>
      <DialogFooter>
        <Button
          variant="dialog"
          size="kira-lg"
          data-dialog-dismiss
          data-testid="ade-repo-gate-cancel"
          @click="gate.cancel()"
        >
          {{ t('common.action.cancel.label') }}
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="chosen.length === 0"
          :pending="gate.pending"
          data-testid="ade-repo-gate-continue"
          @click="gate.confirm(chosen)"
        >
          {{ t('common.continue.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
