<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { useAgentInfo, useAgentList } from '@workbench/agent/agentInfo';
import { computed } from 'vue';
import type { Gate, LaunchTarget } from '../board/taskLaunch';
import { useTaskLaunch } from './useTaskLaunch';

// The plus on a task's Sessions tab bar: per worktree, a terminal (Git module) or an agent session with no first message.
const props = defineProps<{ taskId: string; branchId?: string }>();

const { info } = useAgentInfo();
const { list } = useAgentList();
const { targets, pending, openTerminal, startAgent } = useTaskLaunch(
  () => props.taskId,
  () => props.branchId,
);

// Two or more installed providers: one entry per provider; one keeps today's menu.
const providers = computed(() => list.value.filter((e) => e.available));
const choose = computed(() => providers.value.length > 1);
const usable = computed(() => targets.value.some((x) => x.worktree !== ''));
const several = computed(() => targets.value.length > 1);
const agentLabelOf = (product: string): string =>
  t('space.ade.taskLaunch.agent.label', { agent: product });
const agentLabel = computed(() => agentLabelOf(info.value.productLabel));
const label = (x: LaunchTarget): string =>
  t('common.dotPair.label', { first: x.repo, second: x.branch });
const reason = (g: Gate): string => (g.reason ? t(g.reason) : '');
</script>

<template>
  <DropdownMenu v-if="usable">
    <DropdownMenuTrigger as-child>
      <TooltipIconButton
        icon="add"
        :label="t('space.ade.taskLaunch.trigger.tooltip')"
        class="shrink-0"
        :pending="pending"
        data-testid="ade-task-launch"
      />
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" data-testid="ade-task-launch-menu">
      <template v-if="several">
        <DropdownMenuLabel>{{ t('common.module.terminal.label') }}</DropdownMenuLabel>
        <DropdownMenuItem
          v-for="x in targets"
          :key="`t-${x.branchId}`"
          :disabled="x.terminal.disabled"
          :data-testid="`ade-task-launch-terminal-${x.branchId}`"
          @select="openTerminal(x)"
        >
          {{ label(x) }}
          <span
            v-if="x.terminal.reason"
            class="ml-auto text-muted-foreground"
            data-testid="ade-task-launch-reason"
            >{{
              reason(x.terminal)
            }}</span
          >
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <template v-if="choose">
          <DropdownMenuSub v-for="p in providers" :key="`p-${p.id}`">
            <DropdownMenuSubTrigger :data-testid="`ade-task-launch-provider-${p.id}`">
              {{ agentLabelOf(p.productLabel) }}
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <DropdownMenuItem
                v-for="x in targets"
                :key="`a-${p.id}-${x.branchId}`"
                :disabled="x.agent.disabled"
                :data-testid="`ade-task-launch-agent-${p.id}-${x.branchId}`"
                @select="startAgent(x, p.id)"
              >
                {{ label(x) }}
                <span
                  v-if="x.agent.reason"
                  class="ml-auto text-muted-foreground"
                  data-testid="ade-task-launch-reason"
                  >{{
                    reason(x.agent)
                  }}</span
                >
              </DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
        </template>
        <template v-else>
          <DropdownMenuLabel>{{ agentLabel }}</DropdownMenuLabel>
          <DropdownMenuItem
            v-for="x in targets"
            :key="`a-${x.branchId}`"
            :disabled="x.agent.disabled"
            :data-testid="`ade-task-launch-agent-${x.branchId}`"
            @select="startAgent(x)"
          >
            {{ label(x) }}
            <span
              v-if="x.agent.reason"
              class="ml-auto text-muted-foreground"
              data-testid="ade-task-launch-reason"
              >{{
                reason(x.agent)
              }}</span
            >
          </DropdownMenuItem>
        </template>
      </template>
      <template v-else>
        <DropdownMenuItem
          v-for="x in targets"
          :key="`t-${x.branchId}`"
          :disabled="x.terminal.disabled"
          :data-testid="`ade-task-launch-terminal-${x.branchId}`"
          @select="openTerminal(x)"
        >
          {{ t('common.module.terminal.label') }}
        </DropdownMenuItem>
        <template v-if="choose">
          <DropdownMenuItem
            v-for="p in providers"
            :key="`a-${p.id}`"
            :disabled="targets[0]?.agent.disabled"
            :data-testid="`ade-task-launch-agent-${p.id}-${targets[0]?.branchId}`"
            @select="targets[0] && startAgent(targets[0], p.id)"
          >
            {{ agentLabelOf(p.productLabel) }}
            <span
              v-if="targets[0]?.agent.reason"
              class="ml-auto text-muted-foreground"
              data-testid="ade-task-launch-reason"
              >{{
                reason(targets[0].agent)
              }}</span
            >
          </DropdownMenuItem>
        </template>
        <template v-else>
          <DropdownMenuItem
            v-for="x in targets"
            :key="`a-${x.branchId}`"
            :disabled="x.agent.disabled"
            :data-testid="`ade-task-launch-agent-${x.branchId}`"
            @select="startAgent(x)"
          >
            {{ agentLabel }}
            <span
              v-if="x.agent.reason"
              class="ml-auto text-muted-foreground"
              data-testid="ade-task-launch-reason"
              >{{
                reason(x.agent)
              }}</span
            >
          </DropdownMenuItem>
        </template>
      </template>
    </DropdownMenuContent>
  </DropdownMenu>
  <TooltipIconButton
    v-else
    icon="add"
    :label="t('space.ade.taskLaunch.reason.noWorktree.label')"
    class="shrink-0"
    disabled-trigger
    disabled
    data-testid="ade-task-launch"
  />
</template>
