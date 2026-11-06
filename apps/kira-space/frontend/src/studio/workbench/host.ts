import type { AppMode } from '@shared/domain/mode';
import { t } from '@theme/i18n';
import type { TabIconRender, WorkbenchHost } from '@workbench/host';
import { computed } from 'vue';
import { useModeStore } from '../state/mode';
import type { TabRecord } from '../state/tabDomain';
import { useTabIncognitoStore } from '../state/tabIncognito';
import { TAB_KINDS } from '../state/tabKinds';
import { useTabsStore } from '../state/tabs';
import { TAB_VIEWS } from './tabViews';

// The WorkbenchHost instance this app provides once, in App.vue — every field
// ported verbatim from the old workbench/panels/TabStrip.vue's own `colorFor`/`titleFor`/
// `isAttention`/badgeFor(bodies now behind TAB_KINDS itself, reached by the shared component)
// bodies, minus what already lives on TAB_KINDS (title/pinned/menuExtras — the shared TabStrip.vue
// reaches those directly through `host.kinds`).
export function createWorkbenchHost(): WorkbenchHost<AppMode, TabRecord> {
  const modeStore = useModeStore();
  const tabsStore = useTabsStore();
  const tabIncognitoStore = useTabIncognitoStore();

  return {
    activeWorkspace: computed(() => modeStore.active),
    tabs: tabsStore,
    kinds: TAB_KINDS,
    views: TAB_VIEWS,
    iconFor(tab): TabIconRender {
      return { codicon: TAB_KINDS[tab.kind].icon(tab) };
    },
    railColorFor(tab) {
      return TAB_KINDS[tab.kind].railColor(tab);
    },
    tabBadge(tab) {
      return TAB_KINDS[tab.kind].badge?.(tab) ?? null;
    },
    // P71 §5.1: the incognito eye glyph — uniform across every kind, not one kind's own badge.
    tabIndicator(tab) {
      return tabIncognitoStore.isIncognito(tab.id)
        ? { icon: 'eye-closed', tooltip: t('studio.tab.incognito.tooltip') }
        : null;
    },
  };
}
