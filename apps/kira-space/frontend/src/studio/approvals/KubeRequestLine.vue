<script setup lang="ts">
import type { KubeReadPayload } from '@shared/domain/dbmcp';
import { httpMethodToken } from '@shared/domain/http';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { methodTextClass } from '@theme/methodColor';
import { computed } from 'vue';
import { kubeResource, kubeSelectorTokens } from './kubeRequest';
import { PREPARED_MARK } from './preparedValues';

// The read as a kubectl line built from the final args, never by parsing the statement: the verb in the read
// colour, the resource, then one flag per line. An arg the prepare script set carries a dotted underline.
const props = defineProps<{
  payload: Pick<KubeReadPayload, 'operation' | 'request' | 'changed'>;
}>();

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
const MARK = PREPARED_MARK;
const EMPTY = '""';
const OFF = '=false';
// A shell comment marks the cap, the way kubectl users read a note beside a command.
const COMMENT = '#';
// A prepared key marks its token; the spec and the list match on it.
const mark = (key: string) => (changed(key) ? key : undefined);
const maxRows = computed(() =>
  t('studio.dbMcp.approval.kube.rows.status', { count: req.value?.maxRows ?? 0 }),
);
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
          <span
            class="break-all"
            :class="changed('kind') && MARK"
            data-token="resource"
            :data-prepared="mark('kind')"
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
          <span
            class="break-all text-fg"
            :class="changed(NAME_KEY) && MARK"
            data-token="name"
            :data-prepared="mark(NAME_KEY)"
            >{{
              nameText
            }}</span
          >
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
            <span
              class="break-all"
              :class="[TOKEN.value, changed('namespace') && MARK]"
              :data-prepared="mark('namespace')"
              >{{
                req.namespace
              }}</span
            >
          </TooltipTrigger>
          <TooltipContent>{{
            t('studio.dbMcp.approval.kube.changedByScript.tooltip')
          }}</TooltipContent>
        </Tooltip>
      </template>
      <Tooltip v-else :disabled="!changed('namespace')">
        <TooltipTrigger as-child>
          <span
            :class="[TOKEN.flag, changed('namespace') && MARK]"
            :data-prepared="mark('namespace')"
            >{{
              FLAG.all
            }}</span
          >
        </TooltipTrigger>
        <TooltipContent>{{
          t('studio.dbMcp.approval.kube.changedByScript.tooltip')
        }}</TooltipContent>
      </Tooltip>
    </div>
    <div
      v-if="selector.length || changed('labelSelector')"
      class="flex flex-wrap items-baseline gap-x-2 pl-4"
      data-testid="kube-approval-flag"
      data-flag="selector"
    >
      <span :class="TOKEN.flag">{{ FLAG.l }}</span>
      <Tooltip :disabled="!changed('labelSelector')">
        <TooltipTrigger as-child>
          <span
            class="break-words"
            :class="changed('labelSelector') && MARK"
            :data-prepared="mark('labelSelector')"
            ><template v-if="!selector.length"
              ><span :class="TOKEN.value">{{ EMPTY }}</span></template
            ><template v-for="(tk, i) in selector" :key="i"
              ><span :class="TOKEN[tk.kind]"
                >{{ tk.text }}<wbr v-if="tk.kind === 'op' && tk.text === ','"></span
              ></template
            ></span
          >
        </TooltipTrigger>
        <TooltipContent>{{
          t('studio.dbMcp.approval.kube.changedByScript.tooltip')
        }}</TooltipContent>
      </Tooltip>
    </div>
    <div
      v-if="req.container || changed('container')"
      class="flex flex-wrap items-baseline gap-x-2 pl-4"
      data-testid="kube-approval-flag"
      data-flag="container"
    >
      <span :class="TOKEN.flag">{{ FLAG.c }}</span>
      <Tooltip :disabled="!changed('container')">
        <TooltipTrigger as-child>
          <span
            class="break-all"
            :class="[TOKEN.value, changed('container') && MARK]"
            :data-prepared="mark('container')"
            >{{
              req.container || EMPTY
            }}</span
          >
        </TooltipTrigger>
        <TooltipContent>{{
          t('studio.dbMcp.approval.kube.changedByScript.tooltip')
        }}</TooltipContent>
      </Tooltip>
    </div>
    <div
      v-if="payload.operation === 'list' && changed('maxRows')"
      class="pl-4"
      data-testid="kube-approval-flag"
      data-flag="maxRows"
    >
      <TextTooltip :text="t('studio.dbMcp.approval.kube.changedByScript.tooltip')">
        <span class="text-muted-foreground" :class="MARK" data-prepared="maxRows"
          >{{ COMMENT }} {{ maxRows }}</span
        >
      </TextTooltip>
    </div>
    <div v-if="payload.operation === 'logs'" class="flex flex-col pl-4">
      <div
        v-if="req.previous || changed('previous')"
        data-testid="kube-approval-flag"
        data-flag="previous"
      >
        <Tooltip :disabled="!changed('previous')">
          <TooltipTrigger as-child>
            <span
              :class="[TOKEN.flag, changed('previous') && MARK]"
              :data-prepared="mark('previous')"
              >{{ FLAG.previous }}{{ req.previous ? '' : OFF }}</span
            >
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
            <span :class="[TOKEN.value, changed('tail') && MARK]" :data-prepared="mark('tail')">{{
              req.tail
            }}</span>
          </TooltipTrigger>
          <TooltipContent>{{
            t('studio.dbMcp.approval.kube.changedByScript.tooltip')
          }}</TooltipContent>
        </Tooltip>
      </div>
      <div
        v-if="req.timestamps || changed('timestamps')"
        data-testid="kube-approval-flag"
        data-flag="timestamps"
      >
        <Tooltip :disabled="!changed('timestamps')">
          <TooltipTrigger as-child>
            <span
              :class="[TOKEN.flag, changed('timestamps') && MARK]"
              :data-prepared="mark('timestamps')"
              >{{ FLAG.timestamps }}{{ req.timestamps ? '' : OFF }}</span
            >
          </TooltipTrigger>
          <TooltipContent>{{
            t('studio.dbMcp.approval.kube.changedByScript.tooltip')
          }}</TooltipContent>
        </Tooltip>
      </div>
    </div>
  </div>
</template>
