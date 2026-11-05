<script setup lang="ts">
/**
 * "make it impossible to miss" — flagged as needing extra care, so this file is deliberately
 * conservative: it reads nothing but `ops.statusSummary.value.inProgress` (the one place
 * `classifyInProgress`'s precedence table already lives, in `core`), and touches no state of its
 * own beyond which of its three actions is mid-request.
 *
 * Persistent, above the graph, present for exactly as long as `inProgress !== null` — reactive
 * through `OpsState.refreshStatus`'s own `repo.changed` subscription, which is what makes
 * "Continue re-enables without a manual refresh once the last conflict is staged" true: the
 * watcher already fires on an `index` touch (`git add` is exactly that), `refreshStatus` re-reads
 * `unmergedCount`, and this component is a plain `computed` over the result — there is nothing
 * here to explicitly "recheck".
 *
 * `role="status"`, not `role="alert"`: `alert` is for a message that appears and is gone: an
 * assertive region that *stays on screen* for the length of an entire git operation is a
 * screen-reader trap, re-announcing itself on every incidental change unless the AT's own
 * heuristics happen to suppress it. `status` still announces on appearance (the whole point)
 * without demanding attention indefinitely.
 */
import { describeInProgress } from '@kira/git-core';
import { Alert } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';
import { renderText } from '../lib/text.ts';
import type { OpsState } from '../state/ops.ts';

const props = defineProps<{
  ops: OpsState;
}>();

const inProgress = computed(() => props.ops.statusSummary.value?.inProgress ?? null);
const activeAction = ref<'continue' | 'skip' | 'abort' | undefined>(undefined);

const CONTINUE_REASON_ID = 'git-conflict-continue-reason';

async function onContinue(): Promise<void> {
  if (activeAction.value) return;
  activeAction.value = 'continue';
  try {
    await props.ops.continueOp();
  } finally {
    activeAction.value = undefined;
  }
}

async function onAbort(): Promise<void> {
  if (activeAction.value) return;
  activeAction.value = 'abort';
  try {
    await props.ops.abortOp();
  } finally {
    activeAction.value = undefined;
  }
}

/** `docs/plans/P10.md` W13, §7.11's third sequencer verb — rendered only when `canSkip` is true
 *  (cherry-pick and revert only, probe 6), the same gate `core`'s own `InProgressOperation.canSkip`
 *  already computes. */
async function onSkip(): Promise<void> {
  if (activeAction.value) return;
  activeAction.value = 'skip';
  try {
    await props.ops.skipOp();
  } finally {
    activeAction.value = undefined;
  }
}

const PATH_DISPLAY_CAP = 20;
</script>

<template>
  <!-- W20: an opaque Alert surface: a translucent modal-backdrop scrim fails
       contrast over a light theme. -->
  <Alert
    v-if="inProgress"
    variant="warn"
    strip
    class="shrink-0"
    role="status"
    data-testid="conflict-banner"
  >
    <div class="flex items-center gap-1">
      <span>{{ renderText(describeInProgress(inProgress)) }}</span>
      <span v-if="inProgress.unmergedCount > 0" class="text-muted-foreground text-kira-sm">
        {{ t('git.conflict.unresolved.label', { count: inProgress.unmergedCount }) }}
      </span>

      <span class="flex-1"></span>

      <Button
        v-if="inProgress.canContinue"
        variant="toolbar"
        size="kira"
        :pending="activeAction === 'continue'"
        :disabled="
          inProgress.unmergedCount > 0 || activeAction === 'skip' || activeAction === 'abort'
        "
        :aria-describedby="inProgress.unmergedCount > 0 ? CONTINUE_REASON_ID : undefined"
        @click="onContinue"
      >
        {{ t('common.continue.label') }}
      </Button>
      <Button
        v-if="inProgress.canSkip"
        variant="toolbar"
        size="kira"
        :pending="activeAction === 'skip'"
        :disabled="activeAction === 'continue' || activeAction === 'abort'"
        @click="onSkip"
      >
        {{ t('common.skip.label') }}
      </Button>
      <Button
        v-if="inProgress.canAbort"
        variant="toolbar"
        data-tone="error"
        size="kira"
        :pending="activeAction === 'abort'"
        :disabled="activeAction === 'continue' || activeAction === 'skip'"
        @click="onAbort"
      >
        {{ t('common.abort.label') }}
      </Button>
    </div>

    <FieldDescription v-if="inProgress.unmergedCount > 0" :id="CONTINUE_REASON_ID" class="mt-0.5">
      {{
        t(
          inProgress.canSkip
            ? 'git.conflict.resolveOrSkip.description'
            : 'git.conflict.resolve.description',
          { count: inProgress.unmergedCount },
        )
      }}
    </FieldDescription>
    <FieldDescription v-else-if="inProgress.canSkip" class="mt-0.5">
      {{ t('git.conflict.resolved.description') }}
    </FieldDescription>

    <ul
      v-if="inProgress.conflictedPaths.length > 0"
      class="mt-0.5 pl-3 max-h-20 overflow-y-auto font-data text-kira-sm"
    >
      <li v-for="path in inProgress.conflictedPaths.slice(0, PATH_DISPLAY_CAP)" :key="path">
        <code class="font-data">{{ path }}</code>
      </li>
      <li v-if="inProgress.conflictedPaths.length > PATH_DISPLAY_CAP" class="text-muted-foreground">
        {{
          t('common.more.label', { count: inProgress.conflictedPaths.length - PATH_DISPLAY_CAP })
        }}
      </li>
    </ul>
  </Alert>
</template>
