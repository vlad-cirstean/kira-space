import type { ProofStepMeta } from '@shared/domain/proof';
import type { TabLike, TabStripHost } from '@workbench/host';
import { type ComputedRef, computed } from 'vue';
import type { useProofWindowStore } from './state';
import { proofViewIcon } from './viewIcon';

/** A step shown as a tab. `id` is `${rev}:${ordinal}`, unique across both revisions. */
export interface ProofTab extends TabLike {
  kind: 'step';
  state: { rev: number; meta: ProofStepMeta };
}

export function proofTabId(rev: number, ordinal: number): string {
  return `${rev}:${ordinal}`;
}

/** The read-only tab strip contract: no close, no reorder, activation only. */
export function createProofTabHost(
  tabs: ComputedRef<ProofTab[]>,
  activeId: ComputedRef<string | null>,
  store: ReturnType<typeof useProofWindowStore>,
  badge?: (tab: ProofTab) => { icon: string; tooltip: string } | null,
): TabStripHost<'proof', ProofTab> {
  return {
    activeWorkspace: computed(() => 'proof' as const),
    tabs: {
      get activeIdByWorkspace() {
        return { proof: activeId.value };
      },
      tabsForWorkspace: () => tabs.value,
      activateTab: (id) => {
        store.activeId = id;
      },
    },
    kinds: {
      step: {
        title: (tab) => `#${tab.state.meta.ordinal} ${tab.state.meta.description}`,
        menuExtras: () => [],
      },
    },
    iconFor: (tab) => ({ codicon: proofViewIcon(tab.state.meta.view) }),
    railColorFor: () => undefined,
    tabBadge: badge,
  };
}
