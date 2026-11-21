import type { ComputedRef, InjectionKey, Ref } from 'vue';
import { inject } from 'vue';
import type { MenuItem } from './state/contextMenu';
import type { TabKindRegistry, TabViewMap } from './tabs/types';

// The seam MainView/TabStrip/WorkbenchShell/TitleBar/StatusBar share instead
// of importing an app's own `state/tabs`/`state/tabKinds`/`state/mode`(`workspace`) directly — a
// `provide`/`inject` host object (Vue's own mechanism, no dependency) each app builds once, in its
// own `App.vue`, from its real stores/registries.
//
// Two distinct string unions are in play, and keeping them as two type parameters (rather than
// reusing one `K` for both, tried first and rejected once `TabKindRegistry`'s own invariance in its
// kind parameter surfaced a real type error) matters: `WK`, the *workspace* key
// (`AppMode`/`WorkspaceKey` — what `activeWorkspace`/`tabsForWorkspace` scope by) and the tab
// *kind* union, which is never named separately here at all — it's read off `R['kind']` directly,
// since every real `TabRecord` union already carries it as its own discriminant.
export interface TabLike {
  id: string;
  kind: string;
  state: unknown;
  active: boolean;
}

/** The one axis every generic component calls on the tabs store — `activeWorkspace`
 *  scoped, kind-registry aware. Deliberately narrower than `createTabsStore`'s full
 *  `TabsStoreActions<K, R>` (this package's own state factory): a structural subset, so a host
 *  built from either app's real Pinia store instance satisfies it by shape with no adapter. */
export interface WorkbenchTabsHost<WK extends string, R extends TabLike> {
  readonly tabs: readonly R[];
  readonly activeIdByWorkspace: Record<WK, string | null>;
  isPreview(id: string): boolean;
  tabsForWorkspace(key: WK): R[];
  activateTab(id: string): void;
  closeTab(id: string): void;
  closeOthers(id: string): void;
  closeToTheRight(id: string): void;
  closeAll(key: WK): void;
  duplicateTab(id: string): unknown;
  moveTab(fromId: string, toId: string): void;
  promoteTab(id: string): void;
}

/** A tab's icon, resolved into the one shape `TabStrip.vue` renders — Kira Studio's own
 *  `TabIcon` (`string`, a codicon name) always resolves to `{ codicon }`; Kira Space's
 *  (`string | { filePath }`) resolves either branch, `{ fileStyle }` mirroring
 *  `RepoTreeRow.vue`'s own seti-icon `mask-image` style object for a `repo-file` tab. */
export type TabIconRender = { codicon: string } | { fileStyle: Record<string, string> };

/** What `TabStrip.vue` reads of a kind. A capability a host omits hides the matching affordance. */
export interface TabStripKind<R extends TabLike> {
  pinned?: true;
  title(tab: R): string;
  menuExtras(tab: R): MenuItem[];
}

export interface TabStripTabs<WK extends string, R extends TabLike> {
  readonly activeIdByWorkspace: Record<WK, string | null>;
  tabsForWorkspace(key: WK): R[];
  activateTab(id: string): void;
  isPreview?(id: string): boolean;
  promoteTab?(id: string): void;
  closeTab?(id: string): void;
  closeOthers?(id: string): void;
  closeToTheRight?(id: string): void;
  closeAll?(key: WK): void;
  duplicateTab?(id: string): unknown;
  moveTab?(fromId: string, toId: string): void;
}

/** Where a tab dropped outside the window landed, in client coordinates (CSS px). */
export interface TabDropPoint {
  clientX: number;
  clientY: number;
  grabX: number;
  grabY: number;
  innerWidth: number;
  innerHeight: number;
}

/** Moving a tab to another window (P722). A strip given none behaves as before. */
export interface TabWindowMove<R extends TabLike> {
  /** Whether this tab may leave the window at all. */
  canMove(tab: R): boolean;
  /** Whether dragging a tab out of the window works here (false on Linux Wayland). */
  readonly dragOut: Readonly<Ref<boolean>>;
  /** The other live windows, numbered as the tab menu names them. */
  readonly targets: Readonly<Ref<readonly { key: string; number: number }[]>>;
  move(
    tab: R,
    to: { newWindow: true } | { target: string } | { drop: TabDropPoint },
  ): Promise<void>;
}

/** The narrow seam `TabStrip.vue` consumes: `WorkbenchHost` extends it, and a module with its own
 *  tab model (ade) implements only this. */
export interface TabStripHost<WK extends string, R extends TabLike> {
  readonly activeWorkspace: ComputedRef<WK>;
  readonly tabs: TabStripTabs<WK, R>;
  readonly kinds: { readonly [kind: string]: TabStripKind<R> | undefined };
  iconFor(tab: R): TabIconRender;
  railColorFor(tab: R): string | undefined;
  /** Tear-off and "Move to window"; absent hides both. */
  windowMove?: TabWindowMove<R>;
  /** A trailing mark after the tab title — Kira Studio's kind-supplied `badge()` member
   *  (`TAB_KINDS[tab.kind].badge`); Kira Space wires none, since no kind of its own declares one. */
  tabBadge?(tab: R): { icon: string; tooltip: string } | null;
  /** A leading mark before the tab title, the one per-tab extra neither `iconFor` (the tab's own
   *  icon) nor `tabBadge` (a trailing, kind-supplied mark) covers — Kira Studio's own incognito eye
   *  glyph, uniform across every kind rather than supplied by one kind's own registry entry. */
  tabIndicator?(tab: R): { icon: string; tooltip: string } | null;
}

export interface WorkbenchHost<WK extends string, R extends TabLike> extends TabStripHost<WK, R> {
  readonly tabs: WorkbenchTabsHost<WK, R>;
  readonly kinds: TabKindRegistry<R['kind'], R, unknown, unknown, MenuItem>;
  readonly views: TabViewMap<R['kind']>;
}

// An InjectionKey carries one concrete type; each app's real WK/R instantiation is narrowed back
// at useWorkbenchHost's own call site below, the same "cast at the provide/inject boundary" shape
// Vue's own generic-provide docs use — there is no way to spell "any instantiation of a generic
// interface" as the key's own type parameter.
// biome-ignore lint/suspicious/noExplicitAny: see above.
type AnyWorkbenchHost = WorkbenchHost<any, any>;

export const workbenchHostKey = Symbol('workbenchHost') as InjectionKey<AnyWorkbenchHost>;

export function useWorkbenchHost<
  WK extends string = string,
  R extends TabLike = TabLike,
>(): WorkbenchHost<WK, R> {
  const host = inject(workbenchHostKey);
  if (!host) {
    throw new Error('useWorkbenchHost() called with no WorkbenchHost provided (App.vue)');
  }
  return host as WorkbenchHost<WK, R>;
}
