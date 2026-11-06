import {
  type KibanaConsoleState,
  type KibanaDiscoverBody,
  type KibanaSavedLink,
  kibanaConsoleStateSchema,
  kibanaDiscoverBodySchema,
} from '@shared/domain/kibana';
import type { SavedDiscoverQuery } from '@shared/domain/queries';
import type { ConsoleTabRecord, ConsoleTabState } from '../../../state/tabDomain';
import { useTabsStore } from '../../../state/tabs';
import { setKibana } from '../state';

/** The whole Discover state of a tab as a saved body; null while no data view is chosen. */
export function discoverBody(text: string, k: KibanaConsoleState): KibanaDiscoverBody | null {
  if (!k.dataViewId) return null;
  return {
    query: text,
    dataViewId: k.dataViewId,
    dataViewName: k.dataViewName,
    language: k.language,
    time: k.time,
    filters: k.filters,
    columns: k.columns,
    sort: k.sort,
    sampleSize: k.sampleSize,
    interval: k.interval,
  };
}

// Parsing fixes key order; the view name is display only.
function canonical(body: KibanaDiscoverBody): string {
  return JSON.stringify(kibanaDiscoverBodySchema.parse({ ...body, dataViewName: null }));
}

function sameDiscoverBody(a: KibanaDiscoverBody, b: KibanaDiscoverBody): boolean {
  return canonical(a) === canonical(b);
}

/** Pure over tab state, so it holds for unmounted tabs and across restarts. */
export function discoverDirty(state: ConsoleTabState | undefined): boolean {
  const k = state?.kibana;
  if (!state || !k?.saved) return false;
  const current = discoverBody(state.text, k);
  return current === null || !sameDiscoverBody(current, k.saved.body);
}

/** Inverse of discoverBody: the tab state a saved query restores, linked to it. */
export function kibanaStateFromBody(
  body: KibanaDiscoverBody,
  saved: { id: string; name: string },
): KibanaConsoleState {
  return kibanaConsoleStateSchema.parse({
    dataViewId: body.dataViewId,
    dataViewName: body.dataViewName,
    language: body.language,
    time: body.time,
    filters: body.filters,
    columns: body.columns,
    sort: body.sort,
    sampleSize: body.sampleSize,
    interval: body.interval,
    saved: { id: saved.id, name: saved.name, body },
  });
}

export function savedLink(entry: SavedDiscoverQuery): KibanaSavedLink {
  return { id: entry.id, name: entry.name, body: entry.body };
}

function linkedTabs(id: string): ConsoleTabRecord[] {
  return useTabsStore().tabs.flatMap((tab) =>
    tab.kind === 'console' && tab.state.kibana?.saved?.id === id ? [tab] : [],
  );
}

/** A rename of the saved query shows on every tab linked to it. */
export function relinkName(id: string, name: string): void {
  for (const tab of linkedTabs(id)) {
    const k = tab.state.kibana;
    if (k?.saved) setKibana(tab.id, { ...k, saved: { ...k.saved, name } });
  }
}

/** The saved query is gone: its tabs keep their state and become unsaved. */
export function unlink(id: string): void {
  for (const tab of linkedTabs(id)) {
    const k = tab.state.kibana;
    if (k) setKibana(tab.id, { ...k, saved: null });
  }
}
