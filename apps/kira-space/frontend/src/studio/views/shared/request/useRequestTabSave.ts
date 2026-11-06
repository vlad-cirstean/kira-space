export interface UseRequestTabSaveOptions<T> {
  tabId: () => string;
  incognito: () => boolean;
  itemId: () => string | null;
  /** The tab's own currently-saved record, or null when it isn't bound to one (a fresh tab, or
   *  D14's orphan case — a row deleted in this window or another). Presence alone decides Save vs.
   *  Save as…, its own content is never read here. */
  saved: () => unknown | null;
  /** True while a restored tab's saved row is still being fetched — itemId set, no cache
   *  entry yet, and not yet confirmed orphaned either. `saved()` reads null for this exact same
   *  reason a genuinely deleted row's does; without this, onSave took the two as the same thing
   *  and routed Save into Save as…, silently creating a duplicate row and rebinding the tab to it.
   *  Optional: a caller with nothing async to resolve (there is none today) omits it. */
  unresolved?: () => boolean;
  name: () => string;
  toSaved: () => T;
  save: (itemId: string, name: string, body: T) => void | Promise<void>;
  openSaveDialog: (tabId: string, name: string, body: T) => void;
}

/** HttpRequestView.vue's and GrpcRequestView.vue's own onSave/onSaveAs pair — byte-identical apart
 *  from which store method/converter each calls. An incognito tab has no route into a persisting
 *  editor — both no-op, which also covers each view's own registerCommand('api.save', onSave) and
 *  the command palette entry it registers. */
export function useRequestTabSave<T>(options: UseRequestTabSaveOptions<T>) {
  function onSave(): void {
    if (options.incognito()) return;
    // P108 F4: still finding out whether this itemId is a genuinely saved row or an orphan —
    // wait rather than guessing (the UI disables the Save button for the same reason, but this
    // guard also covers the api.save command/shortcut, which the disabled attribute doesn't).
    if (options.unresolved?.()) return;
    const itemId = options.itemId();
    if (!itemId || options.saved() === null) {
      onSaveAs();
      return;
    }
    void options.save(itemId, options.name(), options.toSaved());
  }

  function onSaveAs(): void {
    if (options.incognito()) return;
    options.openSaveDialog(options.tabId(), options.name(), options.toSaved());
  }

  return { onSave, onSaveAs };
}
