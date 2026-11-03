<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { formatBytes, formatRelative } from '@theme/format';
import { t } from '@theme/i18n';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { control } from '../bridge/control';
import type { IssueAttachment } from './wire';

// Attachments as tiles that open in the OS browser. The files cannot load here: the tracker needs
// credentials Kira never touches, and the page may fetch only its own origin.
defineProps<{ attachments: IssueAttachment[]; truncated: boolean; label: string }>();

function open(a: IssueAttachment): void {
  fireAndForget(control.openExternal(a.url), 'hostView.issue');
}
</script>

<template>
  <section
    v-if="attachments.length > 0"
    class="flex flex-col gap-2"
    data-testid="issue-attachments"
  >
    <SectionHeading
      :label="t('space.hostView.issue.attachments.heading.title')"
      :count="attachments.length"
    />
    <div class="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-2">
      <Tooltip v-for="a in attachments" :key="a.id">
        <TooltipTrigger as-child>
          <Button
            variant="dialog"
            size="inline"
            class="min-w-0 flex-col items-start gap-0.5 px-2 py-1.5 text-left"
            data-testid="issue-attachment"
            @click="open(a)"
          >
            <span class="flex w-full min-w-0 items-center gap-1.5">
              <CodiconIcon
                :name="a.mimeType.startsWith('image/') ? 'file-media' : 'file'"
                :size="13"
              />
              <TruncatedText :text="a.filename" />
            </span>
            <span class="w-full truncate text-kira-sm font-normal text-muted-foreground">
              {{ formatBytes(a.size) }}
              <template v-if="a.author"> · {{ a.author.name }}</template>
              <template v-if="a.createdAt"> · {{ formatRelative(a.createdAt) }}</template>
            </span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>{{
          t('space.ade.board.taskMenu.openHost.label', { label })
        }}</TooltipContent>
      </Tooltip>
    </div>
    <FieldDescription v-if="truncated" data-testid="issue-attachments-truncated">{{
      t('space.hostView.issue.attachments.truncated.hint', { label })
    }}</FieldDescription>
  </section>
</template>
