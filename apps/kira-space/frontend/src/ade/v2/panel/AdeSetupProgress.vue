<script setup lang="ts">
import ScriptProgress from '@theme/components/ScriptProgress.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref } from 'vue';
import { useRepos } from '../../../repo/state/reposQueries';
import { setupStatus } from '../board/setupStatus';
import { useRetrySetup } from '../queries';
import { ACTION_CLASS } from '../tones';
import type { Branch } from '../wire';
import AdeRunLog from './AdeRunLog.vue';

// One branch's prepare-script status: where it is, how long, why it failed, its output and Retry.
// The same block sits in the Details tab, the launch dialogs and the step block.
const props = defineProps<{ branch: Branch; repo: string; compact?: boolean }>();
const repos = useRepos();
const retry = useRetrySetup();
const error = ref('');

const status = computed(() => {
  const setup = props.branch.setup;
  if (!setup) return null;
  const timeout =
    repos.data.value?.repos.find((r) => r.codeRepoId === props.branch.codeRepoId)?.prepareTimeout ??
    '';
  return setupStatus(setup, props.repo, props.branch.name, timeout);
});

async function onRetry(): Promise<void> {
  error.value = '';
  try {
    await retry.mutateAsync({ branchId: props.branch.id });
  } catch (err) {
    error.value = errorText(err);
  }
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
          variant="dialog"
          v-if="status.state === 'failed'"
          size="kira"
          class="shrink-0"
          :class="ACTION_CLASS.amber"
          :pending="retry.isPending.value"
          data-testid="ade-setup-retry"
          @click="onRetry"
        >
          {{ t('space.ade.setup.retry.label') }}
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
