<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { FieldDescription } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import '@vue-flow/core/dist/style.css';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { Input } from '@theme/components/ui/input';
import { ResizeHandle } from '@theme/components/ui/resizable';
import { useLiveSize } from '@theme/components/ui/resizable/useLiveSize';
import { type Connection, type Edge, type Node, useVueFlow, VueFlow } from '@vue-flow/core';
import { useElementSize } from '@vueuse/core';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, onBeforeUnmount, ref, useTemplateRef, watch } from 'vue';
import { kindLabel, PLACEHOLDER_TOKENS, STEP_KINDS } from '../board/workflowForm';
import {
  canMove,
  type EdgeData,
  type GraphEdge,
  type GraphNode,
  layoutWorkflow,
  type StepNodeData,
} from '../board/workflowGraph';
import { useSaveWorkflow } from '../queries';
import { useWorkflows } from '../readQueries';
import { useAdeWorkflowDraftStore } from '../state/adeWorkflowDraft';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import type { Workflow, WorkflowEntry } from '../wire';
import AdeEndNode from './AdeEndNode.vue';
import AdeResultEdge from './AdeResultEdge.vue';
import AdeStepInspector from './AdeStepInspector.vue';
import AdeStepNode from './AdeStepNode.vue';
import AdeWorkflowSaveBar from './AdeWorkflowSaveBar.vue';
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
const { confirmRemoveStep } = useStepRemoval();
const { setViewport } = useVueFlow();

draft.open(props.entry.workflow, props.entry.hash);
watch(
  () => props.entry,
  (e) => draft.push(e.workflow, e.hash),
);
watch(
  () => draft.dirty,
  (v) => {
    wfUi.dirty = v;
  },
);

async function saveNow(): Promise<void> {
  const current = draft.draft;
  if (!draft.dirty || !current) return;
  const sent = JSON.stringify(current);
  try {
    const entry = await save.mutateAsync({
      fileName: props.entry.fileName,
      workflow: current,
      baseHash: draft.baseHash,
    });
    draft.saved(sent, entry.hash);
  } catch (err) {
    draft.error = errorText(err);
    draft.conflict = (err as { code?: string }).code === 'E_CONFLICT';
  }
}
const discard = (): void => draft.discard(props.entry.workflow, props.entry.hash);
function overwrite(): void {
  draft.rebase(props.entry.hash);
  void saveNow();
}

const layout = computed(() =>
  draft.draft ? layoutWorkflow(draft.draft) : { nodes: [], edges: [], width: 0, height: 0 },
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

const flowNodes = computed<Node[]>(() => {
  const wf = draft.draft;
  if (!wf) return [];
  return layout.value.nodes.map((n) => ({
    id: n.id,
    type: n.kind,
    position: { x: n.x, y: n.y },
    draggable: false,
    data: nodeData(n, wf),
  }));
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
    class="flex h-full min-h-0 flex-col gap-3"
    data-testid="ade-wf-form"
  >
    <AdeWorkflowSaveBar
      :dirty="draft.dirty"
      :saving="save.isPending.value"
      @save="saveNow"
      @discard="discard"
    />
    <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
      <Label for="ade-wf-form-name" class="text-kira-sm text-muted-foreground">{{
        t('common.field.name.label')
      }}</Label>
      <Input
        id="ade-wf-form-name"
        :model-value="draft.draft.name"
        size="kira-lg"
        class="w-72 font-medium"
        data-testid="ade-wf-form-name"
        @update:model-value="(v: string | number) => draft.patchWorkflow({ name: String(v) })"
      />
    </div>
    <div ref="split" class="flex min-h-96 flex-1 rounded-kira bg-bg">
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
                    ['ade-arrow-ok', 'fill-tone-green-solid'],
                    ['ade-arrow-fail', 'fill-tone-red-solid'],
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
                  variant="dialog"
                  size="kira-lg"
                  class="bg-transparent"
                  data-testid="ade-wf-add-step"
                >
                  {{ t('space.ade.workflows.graph.addStep.label') }}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="center">
                <DropdownMenuItem
                  v-for="k in STEP_KINDS"
                  :key="k"
                  :data-testid="`ade-wf-add-step-${k}`"
                  @select="draft.createStep(k)"
                >
                  {{ kindLabel(k) }}
                </DropdownMenuItem>
                <template v-if="library.length">
                  <DropdownMenuSeparator />
                  <DropdownMenuLabel>{{
                    t('space.ade.workflows.library.add.label')
                  }}</DropdownMenuLabel>
                  <DropdownMenuItem
                    v-for="s in library"
                    :key="s.id"
                    :data-testid="`ade-wf-add-library-${s.id}`"
                    @select="draft.createFromLibrary(s)"
                  >
                    {{ s.name }}
                  </DropdownMenuItem>
                </template>
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
            :key="draft.step.id"
            :step="draft.step"
            :steps="draft.draft.steps"
          />
          <FieldDescription v-else class="p-3" data-testid="ade-wf-inspector-empty">
            <CatalogText k="space.ade.workflows.graph.inspectorEmpty.emptyHint">
              <template #tokens><span class="font-data">{{ PLACEHOLDER_TOKENS }}</span></template>
            </CatalogText>
          </FieldDescription>
        </div>
      </div>
    </div>
    <Alert v-if="draft.error" variant="error" data-testid="ade-wf-save-error">
      <AlertDescription>{{ draft.error }}</AlertDescription>
      <AlertAction>
        <template v-if="draft.conflict">
          <Button variant="link" size="inline" data-testid="ade-wf-reload" @click="discard">
            {{ t('common.action.refresh.label') }}
          </Button>
          <Button variant="link" size="inline" data-testid="ade-wf-overwrite" @click="overwrite">
            {{ t('space.ade.workflows.conflict.overwrite.label') }}
          </Button>
        </template>
        <Button
          v-else
          variant="link"
          size="inline"
          data-testid="ade-wf-switch-yaml"
          @click="wfUi.workflowMode = 'yaml'"
        >
          {{ t('space.ade.workflows.graph.switchYaml.label') }}
        </Button>
      </AlertAction>
    </Alert>
  </div>
  <EmptyState
    v-else
    fill
    :title="t('space.ade.workflows.graph.noValid.body')"
    data-testid="ade-wf-no-valid"
  >
    <Button variant="link" size="inline" @click="wfUi.workflowMode = 'yaml'">
      {{ t('space.ade.workflows.graph.fixInYaml.label') }}
    </Button>
  </EmptyState>
</template>
