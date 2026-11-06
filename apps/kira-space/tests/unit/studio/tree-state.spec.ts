// P2 R1: two related fixes to project/state/tree.ts.
//
// 1. treeState used to be one deep reactive() wrapping `children` — a Record<rowKey, TreeNode[]>
//    that's the tree's own node cache. A big expanded schema or a Redis namespace can cache
//    thousands of TreeNode objects (plus their badges arrays) there; deep reactive() wrapped every
//    one of them in a Proxy for no reason, since the only two writers (loadChildren,
//    dropConnectionState) always replace or delete a whole rowKey entry, never mutate a cached
//    node's own field. `children` is now its own nested shallowReactive — test 1 confirms that
//    directly, test 2 confirms the "whole entry replace/delete" contract that actually matters is
//    still tracked. Test 3 guards the part that's easy to break doing this: `expanded`/`loading`
//    are Sets mutated in place (.add()/.delete()), which only stay tracked under *deep* reactivity —
//    so treeState itself must stay a real reactive(), only `children` opts out.
//
// 2. searchResult's buildRows/buildNodeRow recurse over every cached node for every connection on
//    every recompute; SearchBox.vue/ProjectPanel.vue used to write straight into treeState.search
//    on every keystroke with nothing debouncing the recompute this triggers. Test 4 confirms a
//    burst of synchronous writes (a fast typist) produces exactly one settled update, not one per
//    character.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { sleep } from '@workbench/testing/unit/async';
import { setActivePinia } from 'pinia';
import { effect, isShallow } from 'vue';
import { pinia } from '../../../frontend/src/state/pinia';

setActivePinia(pinia);

const { useTreeStore } = await import('../../../frontend/src/studio/project/state/tree');
const treeStore = useTreeStore();

describe('tree state reactivity (P2 R1)', () => {
  test('1. treeStore.children is a shallow-reactive cache, not a deep one', () => {
    expect(isShallow(treeStore.children)).toBe(true);
  });

  test('2. replacing or deleting a whole children[key] entry is still tracked', () => {
    const key = 'conn-shallow-1|';
    let seen: unknown;
    effect(() => {
      seen = treeStore.children[key];
    });
    expect(seen).toBeUndefined();

    treeStore.children[key] = [];
    expect(seen).toEqual([]);

    delete treeStore.children[key];
    expect(seen).toBeUndefined();
  });

  test('3. expanded/loading Sets stay deep-reactive: in-place add()/delete() is tracked', () => {
    const key = 'conn-shallow-2|';
    let expandedSeen = false;
    effect(() => {
      expandedSeen = treeStore.expanded.has(key);
    });
    expect(expandedSeen).toBe(false);

    treeStore.expanded.add(key);
    expect(expandedSeen).toBe(true);

    treeStore.expanded.delete(key);
    expect(expandedSeen).toBe(false);
  });

  test('4. rapid search keystrokes only settle the active query once, after typing pauses', async () => {
    treeStore.search = '';
    await sleep(250);
    expect(treeStore.activeSearchQuery).toBe('');

    for (const partial of ['o', 'or', 'ord', 'orde', 'order']) {
      treeStore.search = partial;
    }
    // Still the pre-burst query immediately after the synchronous writes — the debounce hasn't
    // fired yet, so a full tree recompute hasn't run for any of the five keystrokes.
    expect(treeStore.activeSearchQuery).toBe('');

    // Still unsettled well short of the debounce window — this is the part that actually proves
    // there's a real delay rather than just Vue's own watcher batching (which would already have
    // settled by the next microtask, long before 50ms).
    await sleep(50);
    expect(treeStore.activeSearchQuery).toBe('');

    await sleep(250);
    expect(treeStore.activeSearchQuery).toBe('order');
  });
});
