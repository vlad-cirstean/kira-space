import { guardedDraftState } from '@workbench/behaviour/guardedDraft';
import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { detachedStep, type Overridable, referenceStep, resetKey } from '../board/stepLibrary';
import { cloneWorkflow, runnableSteps, stepIdFor } from '../board/workflowForm';
import {
  addResult,
  addStep,
  clearRoute,
  clearStaleImplementers,
  insertStep,
  moveStep,
  patchResult,
  removeResult,
  removeStep,
  renameStepRefs,
  setRoute,
  setStart,
} from '../board/workflowGraph';
import type { StepKind, StepResult, Workflow, WorkflowStep } from '../wire';

export interface Selection {
  stepId: string;
}

// The graph editor's draft: a local copy of the last valid workflow that every canvas and inspector
// edit changes. Save writes it whole (with the file hash it was built on, so a change made elsewhere
// is refused), Discard drops it; a pushed workflow never replaces unsaved edits.
export const useAdeWorkflowDraftStore = defineStore('adeWorkflowDraft', () => {
  // Draft, the file hash it was built on and dirty live here; `useGuardedDraft` in the saving
  // component drives them (save, Discard, Reload, push, conflict).
  const { draft, baseHash, dirty } = guardedDraftState<Workflow | null>(null);
  const selection = ref<Selection | null>(null);
  /** Bumped when another step is selected, not when the selected step's id follows its name. */
  const selectionKey = ref(0);
  /** Ids of steps added since the draft was opened or last saved: their id still follows the name. */
  const fresh = ref(new Set<string>());

  const step = computed(
    () => draft.value?.steps.find((s) => s.id === selection.value?.stepId) ?? null,
  );
  const canSkip = computed(() => runnableSteps(draft.value?.steps ?? []) > 1);

  function open(wf: Workflow | null, hash = ''): void {
    draft.value = wf ? cloneWorkflow(wf) : null;
    baseHash.value = hash;
    dirty.value = false;
    selection.value = null;
    selectionKey.value++;
    fresh.value = new Set();
  }
  function edit(next: Workflow): void {
    const wf = clearStaleImplementers(next);
    draft.value = wf;
    dirty.value = true;
    const sel = selection.value;
    if (sel && !wf.steps.some((s) => s.id === sel.stepId)) selection.value = null;
  }
  /** After a save: ids of added steps stop following their name. */
  function markSaved(): void {
    fresh.value = new Set();
  }
  function select(sel: Selection | null): void {
    selection.value = sel;
    selectionKey.value++;
  }

  const wf = (): Workflow | null => draft.value;
  function patchWorkflow(p: Partial<Pick<Workflow, 'name'>>): void {
    if (draft.value) edit({ ...draft.value, ...p });
  }
  /** A step's id follows its name until the first save, with every route and implementer naming it. */
  function patchStep(stepId: string, p: Partial<WorkflowStep>): void {
    const w = wf();
    if (!w) return;
    let next: Workflow = {
      ...w,
      steps: w.steps.map((x) => (x.id === stepId ? { ...x, ...p } : x)),
    };
    if (p.name !== undefined && fresh.value.has(stepId)) {
      const id = stepIdFor(
        p.name,
        w.steps.filter((x) => x.id !== stepId).map((x) => x.id),
      );
      if (id !== stepId) {
        next = renameStepRefs(next, stepId, id);
        fresh.value.delete(stepId);
        fresh.value.add(id);
        if (selection.value?.stepId === stepId) selection.value = { stepId: id };
      }
    }
    edit(next);
  }
  /** The step replaced whole, e.g. after a kind change. */
  function replaceStep(next: WorkflowStep): void {
    const w = wf();
    if (w) edit({ ...w, steps: w.steps.map((x) => (x.id === next.id ? next : x)) });
  }
  /** A new step of `kind` at the end, or right after `afterId`; it becomes the selection. */
  function createStep(kind: StepKind, afterId?: string): void {
    const w = wf();
    if (!w) return;
    const r = addStep(w, kind, afterId);
    edit(r.wf);
    fresh.value.add(r.stepId);
    select({ stepId: r.stepId });
  }
  /** A reference to library step `lib` at the end, or right after `afterId`; it becomes the selection. */
  function createFromLibrary(lib: WorkflowStep, afterId?: string): void {
    const w = wf();
    if (!w) return;
    const r = insertStep(
      w,
      (taken) => referenceStep(lib, taken, stepIdFor(lib.id, taken)),
      afterId,
    );
    edit(r.wf);
    select({ stepId: r.stepId });
  }
  /** The step turned into an inline copy of what it is now. */
  function detach(stepId: string): void {
    const w = wf();
    const s = w?.steps.find((x) => x.id === stepId);
    if (s) replaceStep(detachedStep(s));
  }
  /** One overridden field of a reference back at the library step's value. */
  function resetField(stepId: string, lib: WorkflowStep, key: Overridable): void {
    const s = wf()?.steps.find((x) => x.id === stepId);
    if (s) replaceStep(resetKey(s, lib, key));
  }
  function deleteStep(stepId: string): void {
    const w = wf();
    if (w) edit(removeStep(w, stepId));
  }
  function moveStepBy(stepId: string, delta: -1 | 1): void {
    const w = wf();
    if (w) edit(moveStep(w, stepId, delta));
  }
  function makeStart(stepId: string): void {
    const w = wf();
    if (w) edit(setStart(w, stepId));
  }
  function route(stepId: string, resultId: string, target: string): void {
    const w = wf();
    if (w) edit(setRoute(w, stepId, resultId, target));
  }
  function clear(stepId: string, resultIds: string[]): void {
    const w = wf();
    if (w) edit(clearRoute(w, stepId, resultIds));
  }
  function createResult(stepId: string): void {
    const w = wf();
    if (w) edit(addResult(w, stepId));
  }
  function deleteResult(stepId: string, resultId: string): void {
    const w = wf();
    if (w) edit(removeResult(w, stepId, resultId));
  }
  function changeResult(
    stepId: string,
    resultId: string,
    p: Partial<Pick<StepResult, 'id' | 'ok' | 'description'>>,
  ): void {
    const w = wf();
    if (w) edit(patchResult(w, stepId, resultId, p));
  }

  return {
    draft,
    dirty,
    baseHash,
    selection,
    selectionKey,
    step,
    canSkip,
    open,
    markSaved,
    select,
    patchWorkflow,
    patchStep,
    replaceStep,
    createStep,
    createFromLibrary,
    detach,
    resetField,
    moveStepBy,
    deleteStep,
    makeStart,
    route,
    clear,
    createResult,
    deleteResult,
    changeResult,
  };
});
