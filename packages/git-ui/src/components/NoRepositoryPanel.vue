<script setup lang="ts">
import type { RepoCandidate } from '@kira/git-ipc';
/**
 * The "no repository open" state: "the repo picker, prompted, and nothing else." The main
 * content area itself when there is nothing else to show — renders the candidate list inline.
 * The workspace's own folders are the only source of repositories — when none of them
 * is a Git repository, this panel says so plainly instead of offering a folder-picking dialog.
 * This is now the only repo-switch affordance git-ui ships (the toolbar dropdown, which
 * duplicated this list behind a popup, was removed — redundant with Studio's own left sidebar and
 * the one-repo-per-workspace model).
 */
import { TransportError } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { Button } from '@theme/components/ui/button';
import { EmptyDescription } from '@theme/components/ui/empty';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { errorText } from '@workbench/bridge/codedError';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import type { PickerOption } from '@workbench/lib/pickerOptions';
import { computed, onMounted } from 'vue';
import { STATE_ICONS } from '../icons/index.ts';
import type { RepoState } from '../state/repo.ts';

const props = defineProps<{ repoState: RepoState }>();
const emit = defineEmits<(event: 'repoOpened', repoId: string) => void>();

// P108 F8: this panel's own refreshList() call (independent of bootstrap()'s own) can fail too —
// distinct copy so a transport drop here never reads as "none of your folders is a Git
// repository", which is a different, host-confirmed answer this failure never actually gave.
const refresh = useAsyncAction(() => props.repoState.refreshList());
const refreshError = refresh.error;

onMounted(() => void refresh.run());

// P108 F10: this was `@click`-bound directly — Vue's own async-error handling stops the
// rejection from becoming a true unhandled one, but with no `app.config.errorHandler` set it
// never reached the user either.

const candidateOptions = computed<PickerOption[]>(() =>
  props.repoState.candidates.value.map((c: RepoCandidate) => ({
    value: c.path,
    label: c.label,
    detail: c.path,
  })),
);

// `transport-closed` (the host disconnecting mid-request) is hidden outright — this panel
// is about to be replaced by the boot-error banner (F8) rather than needing its own copy of it.
const pick = useAsyncAction(
  async (option: PickerOption) => {
    const result = await props.repoState.open(option.value);
    if (result.kind === 'ok') emit('repoOpened', result.repo.repoId);
  },
  {
    describe: (err) =>
      err instanceof TransportError && err.code === 'transport-closed' ? null : errorText(err),
  },
);
const pickError = pick.error;
</script>

<template>
  <EmptyState
    fill
    data-testid="no-repository-panel"
    :icon="STATE_ICONS.repo"
    :title="t('git.noRepository.open.title')"
  >
    <ProjectPicker
      v-if="repoState.candidates.value.length > 0"
      inline
      class="max-w-105 w-full"
      testid="no-repository"
      :projects="candidateOptions"
      :disabled="repoState.opening.value"
      :placeholder="t('git.noRepository.open.title')"
      @select="pick.run"
    />
    <template v-else>
      <EmptyDescription
        v-if="refreshError"
        class="max-w-105"
        data-testid="no-repository-refresh-error"
      >
        {{ t('git.noRepository.refreshError.error', { error: refreshError }) }}
      </EmptyDescription>
      <EmptyDescription v-else class="max-w-105">
        {{ t('git.noRepository.notRepository.description') }}
      </EmptyDescription>
      <Button
        v-if="refreshError"
        variant="toolbar"
        size="kira"
        data-testid="no-repository-retry"
        @click="refresh.run()"
        ><CodiconIcon :name="ICON.refresh" :size="13" />
        {{ t('common.action.retry.label') }}
      </Button>
    </template>
    <EmptyDescription v-if="pickError" class="max-w-105" data-testid="no-repository-pick-error">
      {{ t('git.noRepository.openError.error', { error: pickError }) }}
    </EmptyDescription>
  </EmptyState>
</template>
