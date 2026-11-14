import type { TabScope } from '@shared/domain/tabs';
import type { Component } from 'vue';

// The generic contract both apps' own `state/tabKinds.ts` instantiate once,
// each with its own kind union `K`, record union `R`, and the three type parameters that
// genuinely differ between them — Kira Studio's `TabIcon` is `string`, Kira Space's is
// `string | { readonly filePath: string }`; Kira Studio's rail colour is `ConnectionColor`, Kira
// Space's is `PaletteColor`; both use the same `MenuItem` shape today but it stays a parameter
// rather than a hard import, so this package never depends on either app's context-menu module
// beyond the generic shape it needs.
export interface TabKindDef<
  K extends string,
  R extends { kind: string; state: unknown },
  Icon,
  Color,
  Menu,
> {
  mode: TabScope;
  title(tab: R): string;
  icon(tab: R): Icon;
  railColor(tab: R): Color | undefined;
  /** A brand-new tab of this kind, opened with nothing to inherit. */
  defaultState(): Extract<R, { kind: K }>['state'];
  /** A restored record's raw `state`, normalized through this kind's own schema — the one place
   *  every per-kind `*TabStateSchema`'s `.default()` fires. `null` means "not
   *  parseable" — every field the schema knows about is missing or malformed, since a schema
   *  whose later fields all carry `.default(...)` already tolerates a row
   *  missing just those. The caller (hydrateTabs) resets that row to `defaultState()` rather than
   *  keeping the unparsed, untrusted raw state. */
  parseState(raw: unknown): Extract<R, { kind: K }>['state'] | null;
  /** "Same target, fresh default state" for most kinds — some keep one field from the source. */
  duplicateState(tab: Extract<R, { kind: K }>): Extract<R, { kind: K }>['state'];
  /** True while the tab holds unsaved work (pending grid edits, a dirty edit buffer); closing it
   *  asks to discard first. Absent for a kind that never holds any. */
  isDirty?(tabId: string): boolean;
  /** Asks this kind's own close question for a dirty tab (Save, Discard, Cancel). True closes it.
   *  Absent: the generic discard confirm asks. Called only when `isDirty` is true. */
  confirmClose?(tabId: string): Promise<boolean>;
  /** Frees whichever page store this kind populated (a no-op miss for a kind with none). */
  dropResources(tabId: string): void;
  /** Appended to the tab strip's own generic context-menu items. */
  menuExtras(tab: R): Menu[];
  /** A small state mark after the tab's title — undefined for a kind with nothing to flag. */
  badge?(tab: R): { icon: string; tooltip: string } | null;
  /** True for exactly one kind per app (Kira Space's `repo-graph`) — tabsForWorkspace's own
   *  stable partition puts every pinned tab of a workspace first, and closeTab/closeOthers/
   *  closeToTheRight/closeAll/duplicateTab/moveTab all guard against it. Absent (not `false`) for
   *  every other kind, so this costs those kinds no line. */
  pinned?: true;
  /** View stays mounted, hidden, from first activation until the tab closes. */
  keepMounted?: true;
}

export type TabKindRegistry<
  K extends string,
  R extends { kind: string; state: unknown },
  Icon,
  Color,
  Menu,
> = {
  readonly [P in K]: TabKindDef<P, R, Icon, Color, Menu>;
};

export type TabViewMap<K extends string> = Record<K, Component>;

// P3 D3: every parseState is a one-liner over the schema its own kind already imports — this is
// the shared shape (safeParse, `.data` on success, `null` on failure) so each registry entry
// states only which schema, not the pattern.
export function parseStateWith<S>(schema: {
  safeParse(raw: unknown): { success: true; data: S } | { success: false };
}): (raw: unknown) => S | null {
  return (raw) => {
    const result = schema.safeParse(raw);
    return result.success ? result.data : null;
  };
}
