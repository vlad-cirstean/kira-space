<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import InlineState from '@theme/components/InlineState.vue';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { paneFrame } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import { useGuardedDraft } from '@workbench/behaviour/guardedDraft';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import MenuItems from '@workbench/components/MenuItems.vue';
import SaveBar from '@workbench/components/SaveBar.vue';
import SaveConflictAlert from '@workbench/config/SaveConflictAlert.vue';
import '@vue-flow/core/dist/style.css';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { Input } from '@theme/components/ui/input';
import { ResizeHandle } from '@theme/components/ui/resizable';
import { useLiveSize } from '@theme/components/ui/resizable/useLiveSize';
import { type Connection, type Edge, type Node, useVueFlow, VueFlow } from '@vue-flow/core';
import { useElementSize } from '@vueuse/core';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import type { MenuItem } from '@workbench/state/contextMenu';
import { storeToRefs } from 'pinia';
import { computed, onBeforeUnmount, ref, shallowRef, useTemplateRef, watch } from 'vue';
import { cloneWorkflow, kindLabel, PLACEHOLDER_TOKENS, STEP_KINDS } from '../board/workflowForm';
import {
  canMove,
  type EdgeData,
  type Graph,
  type GraphEdge,
  type GraphNode,
  type StepNodeData,
} from '../board/workflowGraph';
import { layoutWorkflow, topologyKey } from '../board/workflowLayout';
import { useSaveWorkflow } from '../queries';
import { useWorkflows } from '../readQueries';
import { useAdeWorkflowDraftStore } from '../state/adeWorkflowDraft';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import type { Workflow, WorkflowEntry } from '../wire';
import AdeEndNode from './AdeEndNode.vue';
import AdeResultEdge from './AdeResultEdge.vue';
import AdeStepInspector from './AdeStepInspector.vue';
import AdeStepNode from './AdeStepNode.vue';
import { useStepRemoval } from './useStepRemoval';

// Graph mode: edits a local draft of the last valid workflow as a canvas of steps, one
// connector per step result; Save writes it whole, Discard drops the edits. Layout is automatic and
// locked: top to bottom, one node size, native vertical scroll, no zoom or pan.
const props = defineProps<{ entry: WorkflowEntry }>();
const wfUi = useAdeWorkflowsUiStore();
const draft = useAdeWorkflowDraftStore();
const save = useSaveWorkflow();
const workflows = useWorkflows();
const library = computed(() =>
  (workflows.data.value?.steps ?? []).flatMap((e) => (e.step ? [e.step] : [])),
);
const addStepItems = computed<MenuItem[]>(() => [
  ...STEP_KINDS.map((k) => ({
    type: 'item' as const,
    id: k,
    testId: `ade-wf-add-step-${k}`,
    label: kindLabel(k),
    run: () => draft.createStep(k),
  })),
  ...(library.value.length
    ? [
        { type: 'separator' as const },
        { type: 'label' as const, label: t('space.ade.workflows.library.add.label') },
        ...library.value.map((s) => ({
          type: 'item' as const,
          id: `library-${s.id}`,
          testId: `ade-wf-add-library-${s.id}`,
          label: s.name,
          run: () => draft.createFromLibrary(s),
        })),
      ]
    : []),
]);
const { confirmRemoveStep } = useStepRemoval();
const { setViewport } = useVueFlow();

const { draft: draftRef, baseHash, dirty } = storeToRefs(draft);
const guard = useGuardedDraft<Workflow | null>({
  state: { draft: draftRef, baseHash, dirty },
  source: () =>
    props.entry.workflow ? { value: props.entry.workflow, hash: props.entry.hash } : undefined,
  clone: (w) => (w ? cloneWorkflow(w) : null),
  key: (w) => JSON.stringify(w),
  adopt: (s) => draft.open(s?.value ?? null, s?.hash ?? ''),
  onSaved: draft.markSaved,
  write: async (workflow, hash) => {
    const entry = await save.mutateAsync({
      fileName: props.entry.fileName,
      workflow: workflow as Workflow,
      baseHash: hash,
    });
    if (entry.fileName !== props.entry.fileName) wfUi.workflowFile = entry.fileName;
    return entry;
  },
});
draft.open(props.entry.workflow, props.entry.hash);
watch(
  () => draft.dirty,
  (v) => {
    wfUi.dirty = v;
  },
);

const saveNow = (): Promise<boolean> => guard.save();

// Layout runs only when the topology changes: typing in a step's prompt must not re-run dagre or move
// a node. The watcher reads the draft untracked, so only the key triggers it.
const EMPTY_GRAPH: Graph = { nodes: [], edges: [], width: 0, height: 0 };
const topology = computed(() => (draft.draft ? topologyKey(draft.draft) : ''));
const layout = shallowRef<Graph>(EMPTY_GRAPH);
watch(
  topology,
  () => {
    layout.value = draft.draft ? layoutWorkflow(draft.draft) : EMPTY_GRAPH;
  },
  { immediate: true, flush: 'sync' },
);
const edgeSel = ref('');

function nodeData(n: GraphNode, wf: Workflow): StepNodeData | { w: number; h: number } {
  const base = { w: n.w, h: n.h };
  const index = wf.steps.findIndex((s) => s.id === n.stepId);
  const step = wf.steps[index];
  if (n.kind === 'end' || !step) return base;
  return {
    ...base,
    step,
    index,
    start: index === 0,
    selected: draft.selection?.stepId === step.id,
    canUp: canMove(wf, step.id, -1),
    canDown: canMove(wf, step.id, 1),
    canRemove: wf.steps.length > 1,
    canSkip: draft.canSkip,
  };
}

// A node object is reused while its step and flags are unchanged, so a one-step edit patches one node.
const nodeCache = new Map<string, { sig: string; step: unknown; node: Node }>();
const flowNodes = computed<Node[]>(() => {
  const wf = draft.draft;
  if (!wf) return [];
  const seen = new Set<string>();
  const out = layout.value.nodes.map((n) => {
    const data = nodeData(n, wf);
    const step = 'step' in data ? data.step : undefined;
    const sig = JSON.stringify([n.x, n.y, n.w, n.h, { ...data, step: undefined }]);
    const hit = nodeCache.get(n.id);
    seen.add(n.id);
    if (hit && hit.sig === sig && hit.step === step) return hit.node;
    const node: Node = {
      id: n.id,
      type: n.kind,
      position: { x: n.x, y: n.y },
      draggable: false,
      data,
    };
    nodeCache.set(n.id, { sig, step, node });
    return node;
  });
  for (const id of nodeCache.keys()) if (!seen.has(id)) nodeCache.delete(id);
  return out;
});
const flowEdges = computed<Edge[]>(() =>
  layout.value.edges.map((e) => {
    const fixed = e.results.length === 0;
    const data: EdgeData = {
      tone: e.tone,
      loop: e.loop,
      results: e.results,
      selected: e.id === edgeSel.value,
      spine: e.spine ?? false,
      points: e.points,
      lane: e.lane,
      laneX: e.laneX,
      fixed,
    };
    return {
      id: e.id,
      type: 'result',
      source: e.source,
      target: e.target,
      sourceHandle: fixed ? 'out' : e.sourceHandle,
      targetHandle: e.loop ? 'loop-in' : 'in',
      selectable: false,
      ariaLabel: t('space.ade.workflows.graph.edge.aria', { source: e.source, target: e.target }),
      data,
    };
  }),
);

const byId = (id: string): GraphNode | undefined => layout.value.nodes.find((n) => n.id === id);

function validConnection(c: Connection): boolean {
  const s = byId(c.source);
  const t = byId(c.target);
  return !!s && !!t && s.kind === 'step' && (t.kind === 'step' || t.kind === 'end');
}
function onConnect(c: Connection): void {
  if (!validConnection(c)) return;
  const s = byId(c.source) as GraphNode;
  const t = byId(c.target) as GraphNode;
  const resultId = (c.sourceHandle ?? '').replace(/^r:/, '');
  if (s.stepId && resultId)
    draft.route(s.stepId, resultId, t.kind === 'end' ? 'end' : (t.stepId ?? 'end'));
}
function onNodeClick({ node }: { node: Node }): void {
  const n = byId(node.id);
  edgeSel.value = '';
  if (n?.kind === 'step' && n.stepId) draft.select({ stepId: n.stepId });
}
function onEdgeClick({ edge }: { edge: Edge }): void {
  const e = layout.value.edges.find((x) => x.id === edge.id);
  if (!e?.stepId || e.results.length === 0) return;
  edgeSel.value = e.id;
  draft.select({ stepId: e.stepId });
}
function onPaneClick(): void {
  edgeSel.value = '';
  draft.select(null);
}

const root = useTemplateRef<HTMLElement>('root');
const scroller = useTemplateRef<HTMLElement>('scroller');
const split = useTemplateRef<HTMLElement>('split');
const GRAPH_MIN_PCT = 35;
const GRAPH_MAX_PCT = 78;
const graphPct = ref(66);
const graphPane = useLiveSize(
  () => graphPct.value,
  (pct) => {
    graphPct.value = pct;
  },
);
useShortcut(root, ['form.save'], () => void saveNow());

const PAD = 24;
const { width: paneW } = useElementSize(scroller);
const zoom = computed(() =>
  paneW.value > 0 ? Math.min(1, paneW.value / (layout.value.width + PAD * 2)) : 1,
);
const viewport = computed(() => ({
  x: Math.max(0, (paneW.value - layout.value.width * zoom.value) / 2),
  y: PAD * zoom.value,
  zoom: zoom.value,
}));
const contentH = computed(() => (layout.value.height + PAD * 2) * zoom.value);
watch(viewport, (v) => void setViewport(v), { immediate: true });

async function deleteSelection(): Promise<void> {
  const edge: GraphEdge | undefined = layout.value.edges.find((x) => x.id === edgeSel.value);
  const sel = draft.selection;
  if (edge?.stepId) {
    draft.clear(edge.stepId, edge.results);
    edgeSel.value = '';
  } else if (sel && (draft.draft?.steps.length ?? 0) > 1) {
    await confirmRemoveStep(sel.stepId);
  }
}
useShortcut(root, ['tree.delete'], () => fireAndForget(deleteSelection(), 'adeWorkflows'), {
  ignoreTextFields: true,
});

onBeforeUnmount(() => {
  wfUi.dirty = false;
  draft.open(null);
});
</script>

<template>
  <div
    v-if="draft.draft"
    ref="root"
    class="@container flex h-full min-h-0 flex-col gap-3"
    data-testid="ade-wf-form"
  >
    <SaveBar
      :dirty="draft.dirty"
      :saving="guard.saving.value"
      testid-prefix="ade-wf"
      @save="saveNow"
      @discard="guard.discard"
    >
      <template #start>{{ t('space.ade.workflows.saveBar.applies.hint') }}</template>
    </SaveBar>
    <Field orientation="responsive">
      <FieldLabel for="ade-wf-form-name">{{ t('common.field.name.label') }}</FieldLabel>
      <Input
        id="ade-wf-form-name"
        :model-value="draft.draft.name"
        class="w-56 font-medium"
        data-testid="ade-wf-form-name"
        @update:model-value="(v: string | number) => draft.patchWorkflow({ name: String(v) })"
      />
    </Field>
    <div ref="split" :class="cn(paneFrame, 'flex min-h-96 flex-1')">
      <div class="relative min-w-0" :style="{ flex: `0 0 ${graphPane.size.value}%` }">
        <div
          ref="scroller"
          class="absolute inset-0 overflow-y-auto overflow-x-hidden"
          data-testid="ade-wf-graph"
        >
          <div class="relative min-h-full w-full" :style="{ height: `${contentH}px` }">
            <svg class="absolute size-0" aria-hidden="true">
              <defs>
                <marker
                  v-for="m in [
                    ['ade-arrow-ok', 'fill-ok'],
                    ['ade-arrow-fail', 'fill-error'],
                    ['ade-arrow-neutral', 'fill-muted-foreground'],
                  ]"
                  :id="m[0]"
                  :key="m[0]"
                  viewBox="0 0 10 10"
                  refX="9"
                  refY="5"
                  markerWidth="9"
                  markerHeight="9"
                  orient="auto-start-reverse"
                  markerUnits="userSpaceOnUse"
                >
                  <path d="M 0 0 L 10 5 L 0 10 z" :class="m[1]" />
                </marker>
              </defs>
            </svg>
            <VueFlow
              :nodes="flowNodes"
              :edges="flowEdges"
              :nodes-draggable="false"
              :delete-key-code="null"
              :min-zoom="0.1"
              :max-zoom="1"
              :default-viewport="viewport"
              :zoom-on-scroll="false"
              :zoom-on-pinch="false"
              :zoom-on-double-click="false"
              :pan-on-drag="false"
              :pan-on-scroll="false"
              :prevent-scrolling="false"
              :auto-pan-on-connect="false"
              :auto-pan-on-node-drag="false"
              :is-valid-connection="validConnection"
              @connect="onConnect"
              @node-click="onNodeClick"
              @edge-click="onEdgeClick"
              @pane-click="onPaneClick"
            >
              <template #node-step="p"><AdeStepNode :data="p.data" /></template>
              <template #node-end="p"><AdeEndNode :data="p.data" /></template>
              <template #edge-result="p"><AdeResultEdge v-bind="p" /></template>
            </VueFlow>
          </div>
          <div class="flex justify-center pb-4">
            <DropdownMenu>
              <DropdownMenuTrigger as-child>
                <Button
                  variant="toolbar"
                  size="kira"
                  class="bg-transparent"
                  data-testid="ade-wf-add-step"
                  ><CodiconIcon name="add" :size="13" />
                  {{ t('space.ade.workflows.graph.addStep.label') }}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="center">
                <MenuItems :items="addStepItems" />
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </div>
      <ResizeHandle
        :label="t('space.ade.workflows.graph.resize.aria')"
        line
        unit="percent"
        :container="split"
        :min="GRAPH_MIN_PCT"
        :max="GRAPH_MAX_PCT"
        v-bind="graphPane.bind"
      />
      <div class="min-w-0 flex-1">
        <div class="flex h-full min-h-0 flex-col" data-testid="ade-wf-inspector">
          <AdeStepInspector
            v-if="draft.step"
            :key="draft.selectionKey"
            :step="draft.step"
            :steps="draft.draft.steps"
          />
          <InlineState kind="empty" v-else data-testid="ade-wf-inspector-empty">
            <CatalogText k="space.ade.workflows.graph.inspectorEmpty.emptyHint">
              <template #tokens><span class="font-data">{{ PLACEHOLDER_TOKENS }}</span></template>
            </CatalogText>
          </InlineState>
        </div>
      </div>
    </div>
    <SaveConflictAlert
      :error="guard.saveError.value ?? ''"
      :conflict="guard.conflict.value"
      testid-prefix="ade-wf"
      @reload="guard.reload"
      @overwrite="guard.overwrite"
    >
      <template #action>
        <Button
          variant="link"
          size="inline"
          data-testid="ade-wf-switch-yaml"
          @click="wfUi.workflowMode = 'yaml'"
        >
          {{ t('space.ade.workflows.graph.switchYaml.label') }}
        </Button>
      </template>
    </SaveConflictAlert>
  </div>
  <EmptyState
    v-else
    fill
    icon="type-hierarchy"
    :title="t('space.ade.workflows.graph.noValid.body')"
    data-testid="ade-wf-no-valid"
  >
    <Button variant="link" size="inline" @click="wfUi.workflowMode = 'yaml'">
      {{ t('space.ade.workflows.graph.fixInYaml.label') }}
    </Button>
  </EmptyState>
</template>
