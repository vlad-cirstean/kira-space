<script setup lang="ts">
import type { KubeReadPayload } from '@shared/domain/dbmcp';
import { httpMethodToken } from '@shared/domain/http';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { methodTextClass } from '@theme/methodColor';
import { computed } from 'vue';
import { kubeResource, kubeSelectorTokens } from './kubeRequest';

// The read as a kubectl line built from the final args, never by parsing the statement: the verb in the read
// colour, the resource, then one flag per line. An arg the prepare script set carries a dotted underline.
const props = defineProps<{ payload: KubeReadPayload }>();

const req = computed(() => props.payload.request);
const verb = computed(() => (props.payload.operation === 'list' ? 'get' : props.payload.operation));
const readClass = methodTextClass(httpMethodToken('GET'));
const resource = computed(() => kubeResource(req.value?.kind ?? ''));
const changed = (key: string) => props.payload.changed.includes(key);
const NAME_KEY = computed(() => (props.payload.operation === 'logs' ? 'pod' : 'name'));
const nameText = computed(() =>
  props.payload.operation === 'logs' ? (req.value?.pod ?? '') : (req.value?.name ?? ''),
);
const selector = computed(() => kubeSelectorTokens(req.value?.labelSelector ?? ''));

// kubectl's own words, not copy.
const FLAG = {
  kubectl: 'kubectl',
  n: '-n',
  all: '-A',
  l: '-l',
  c: '-c',
  previous: '--previous',
  tail: '--tail',
  timestamps: '--timestamps',
} as const;
const MARK = 'underline decoration-dotted decoration-warn underline-offset-4';
const TOKEN = {
  flag: 'text-syntax-meta',
  key: 'text-syntax-property',
  op: 'text-syntax-meta',
  value: 'text-syntax-string',
} as const;
</script>

<template>
  <div
    v-if="req"
    class="flex flex-col font-data text-kira-md leading-normal"
    data-testid="kube-approval-line"
  >
    <div class="flex flex-wrap items-baseline gap-x-2" data-testid="kube-approval-command">
      <span class="text-muted-foreground">{{ FLAG.kubectl }}</span>
      <span class="font-semibold" :class="readClass" data-token="verb">{{ verb }}</span>
      <Tooltip :disabled="!changed('kind')">
        <TooltipTrigger as-child>
          <span class="break-all" :class="changed('kind') && MARK" data-token="resource"
            ><span class="font-semibold text-fg">{{ resource.name }}</span
            ><span v-if="resource.group" class="text-muted-foreground"
              >.{{ resource.group }}</span
            ></span
          >
        </TooltipTrigger>
        <TooltipContent>{{
          t('studio.dbMcp.approval.kube.changedByScript.tooltip')
        }}</TooltipContent>
      </Tooltip>
      <Tooltip v-if="nameText" :disabled="!changed(NAME_KEY)">
        <TooltipTrigger as-child>
          <span class="break-all text-fg" :class="changed(NAME_KEY) && MARK" data-token="name">{{
            nameText
          }}</span>
        </TooltipTrigger>
        <TooltipContent>{{
          t('studio.dbMcp.approval.kube.changedByScript.tooltip')
        }}</TooltipContent>
      </Tooltip>
    </div>
    <div
      v-if="req.namespace || payload.operation === 'list'"
      class="flex flex-wrap items-baseline gap-x-2 pl-4"
      data-testid="kube-approval-flag"
      data-flag="namespace"
    >
      <template v-if="req.namespace">
        <span :class="TOKEN.flag">{{ FLAG.n }}</span>
        <Tooltip :disabled="!changed('namespace')">
          <TooltipTrigger as-child>
            <span class="break-all" :class="[TOKEN.value, changed('namespace') && MARK]">{{
              req.namespace
            }}</span>
          </TooltipTrigger>
          <TooltipContent>{{
            t('studio.dbMcp.approval.kube.changedByScript.tooltip')
          }}</TooltipContent>
        </Tooltip>
      </template>
      <Tooltip v-else :disabled="!changed('namespace')">
        <TooltipTrigger as-child>
          <span :class="[TOKEN.flag, changed('namespace') && MARK]">{{ FLAG.all }}</span>
        </TooltipTrigger>
        <TooltipContent>{{
          t('studio.dbMcp.approval.kube.changedByScript.tooltip')
        }}</TooltipContent>
      </Tooltip>
    </div>
    <div
      v-if="selector.length"
      class="flex flex-wrap items-baseline gap-x-2 pl-4"
      data-testid="kube-approval-flag"
      data-flag="selector"
    >
      <span :class="TOKEN.flag">{{ FLAG.l }}</span>
      <Tooltip :disabled="!changed('labelSelector')">
        <TooltipTrigger as-child>
          <span class="break-all" :class="changed('labelSelector') && MARK"
            ><template v-for="(tk, i) in selector" :key="i"
              ><span :class="TOKEN[tk.kind]">{{ tk.text }}</span></template
            ></span
          >
        </TooltipTrigger>
        <TooltipContent>{{
          t('studio.dbMcp.approval.kube.changedByScript.tooltip')
        }}</TooltipContent>
      </Tooltip>
    </div>
    <div
      v-if="req.container"
      class="flex flex-wrap items-baseline gap-x-2 pl-4"
      data-testid="kube-approval-flag"
      data-flag="container"
    >
      <span :class="TOKEN.flag">{{ FLAG.c }}</span>
      <Tooltip :disabled="!changed('container')">
        <TooltipTrigger as-child>
          <span class="break-all" :class="[TOKEN.value, changed('container') && MARK]">{{
            req.container
          }}</span>
        </TooltipTrigger>
        <TooltipContent>{{
          t('studio.dbMcp.approval.kube.changedByScript.tooltip')
        }}</TooltipContent>
      </Tooltip>
    </div>
    <div v-if="payload.operation === 'logs'" class="flex flex-col pl-4">
      <div v-if="req.previous" data-testid="kube-approval-flag" data-flag="previous">
        <Tooltip :disabled="!changed('previous')">
          <TooltipTrigger as-child>
            <span :class="[TOKEN.flag, changed('previous') && MARK]">{{ FLAG.previous }}</span>
          </TooltipTrigger>
          <TooltipContent>{{
            t('studio.dbMcp.approval.kube.changedByScript.tooltip')
          }}</TooltipContent>
        </Tooltip>
      </div>
      <div
        class="flex flex-wrap items-baseline gap-x-2"
        data-testid="kube-approval-flag"
        data-flag="tail"
      >
        <span :class="TOKEN.flag">{{ FLAG.tail }}</span>
        <Tooltip :disabled="!changed('tail')">
          <TooltipTrigger as-child>
            <span :class="[TOKEN.value, changed('tail') && MARK]">{{ req.tail }}</span>
          </TooltipTrigger>
          <TooltipContent>{{
            t('studio.dbMcp.approval.kube.changedByScript.tooltip')
          }}</TooltipContent>
        </Tooltip>
      </div>
      <div v-if="req.timestamps" data-testid="kube-approval-flag" data-flag="timestamps">
        <Tooltip :disabled="!changed('timestamps')">
          <TooltipTrigger as-child>
            <span :class="[TOKEN.flag, changed('timestamps') && MARK]">{{ FLAG.timestamps }}</span>
          </TooltipTrigger>
          <TooltipContent>{{
            t('studio.dbMcp.approval.kube.changedByScript.tooltip')
          }}</TooltipContent>
        </Tooltip>
      </div>
    </div>
  </div>
</template>
