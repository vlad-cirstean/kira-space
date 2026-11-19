<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import ScriptProgress from '@theme/components/ScriptProgress.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useMutationAction } from '@workbench/behaviour/asyncAction';
import { computed } from 'vue';
import { useRepos } from '../../../repo/state/reposQueries';
import { setupStatus } from '../board/setupStatus';
import { useRetrySetup } from '../queries';
import type { Branch } from '../wire';
import AdeRunLog from './AdeRunLog.vue';

// One branch's prepare-script status: where it is, how long, why it failed, its output and Retry.
// The same block sits in the Details tab, the launch dialogs and the step block.
const props = defineProps<{ branch: Branch; repo: string; compact?: boolean }>();
const repos = useRepos();
const retry = useRetrySetup();
const retrying = useMutationAction(retry);
const error = retrying.error;

const status = computed(() => {
  const setup = props.branch.setup;
  if (!setup) return null;
  const timeout =
    repos.data.value?.repos.find((r) => r.codeRepoId === props.branch.codeRepoId)?.prepareTimeout ??
    '';
  return setupStatus(setup, props.repo, props.branch.name, timeout);
});

async function onRetry(): Promise<void> {
  await retrying.run({ branchId: props.branch.id });
}
</script>

<template>
  <div
    v-if="status"
    class="flex flex-col gap-1"
    data-testid="ade-setup-progress"
    :data-branch-id="branch.id"
  >
    <ScriptProgress v-bind="status">
      <template #actions>
        <Button
          variant="toolbar"
          v-if="status.state === 'failed'"
          size="kira"
          class="shrink-0"
          :pending="retry.isPending.value"
          data-testid="ade-setup-retry"
          @click="onRetry"
        >
          <CodiconIcon name="refresh" :size="13" />{{ t('space.ade.setup.retry.label') }}
        </Button>
      </template>
      <AdeRunLog
        v-if="!compact && (status.state === 'running' || status.state === 'failed')"
        kind="setup"
        :id="branch.id"
        max-height="160px"
      />
    </ScriptProgress>
    <Alert v-if="error" variant="error" data-testid="ade-setup-error"
      ><AlertDescription>{{ error }}</AlertDescription></Alert
    >
  </div>
</template>
