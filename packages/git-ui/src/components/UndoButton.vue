<script setup lang="ts">
import { shortSha } from '@kira/git-core';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Button } from '@theme/components/ui/button';
/**
 * The single-level "Undo last operation". Present only while
 * `ops.undoSlot` holds a record (`OpsState.refreshUndo`/`#applyResult` are the only things that
 * ever set it, and both clear it — to `null` — the moment a non-undoable op runs, which is what
 * makes "no non-undoable operation ever renders this" true without this file re-deriving
 * `UNDO_POLICY` itself). Recovery sha is real, copyable text (`clipboardActions.ts`, matching
 * every other sha in this app), never only inside a tooltip.
 *
 * The tooltip text itself moved into `composeUndoTooltip` (mode-matched for a reset) rather than
 * staying the single fixed string this file used to render inline.
 */
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import Identifier from '@workbench/components/Identifier.vue';
import { composeUndoTooltip } from '../state/liveAnnouncements.ts';
import type { OpsState } from '../state/ops.ts';

const props = defineProps<{
  ops: OpsState;
  copy: (text: string, message: string) => void;
}>();

async function undo(): Promise<void> {
  await props.ops.undo();
}
</script>

<template>
  <div v-if="ops.undoSlot.value" class="flex items-center gap-0.5">
    <TextTooltip :text="composeUndoTooltip(ops.undoSlot.value.label)">
      <Button variant="toolbar" size="kira" :pending="ops.busy.value" @click="undo">
        <CodiconIcon name="discard" :size="13" />
        {{ ops.undoSlot.value.label }}
      </Button>
    </TextTooltip>
    <!-- The slot holds real multi-character sha text, not an icon, so an icon-only button's fixed
         square width would clip it. Dropped to the plain default, the same call the
         "Show more"/"Show less" toggle gets for the identical reason. -->
    <TextTooltip
      :text="t('git.undo.copyRecovery.tooltip', { sha: ops.undoSlot.value.recoverySha })"
    >
      <Button
        variant="toolbar"
        size="kira"
        class="text-kira-sm"
        @click="copy(ops.undoSlot.value.recoverySha, t('git.copy.recoverySha.announce'))"
        ><CodiconIcon :name="ICON.copy" :size="13" />
        <Identifier :value="ops.undoSlot.value.recoverySha" :short="shortSha" />
      </Button>
    </TextTooltip>
  </div>
</template>
