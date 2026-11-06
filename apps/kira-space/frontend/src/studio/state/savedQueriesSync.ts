import { queryClient } from '@workbench/state/queryClient';
import { control } from '../bridge/control';

// Saved-query menus read once and stay fresh until invalidated, so a write must announce itself: Go emits
// kira:savedQueries:changed after each successful write and kira:filterHistory:changed after
// each recorded filter, from this window or any other. The kind prefixes mirror each menu's key.
let unsubscribeSaved: (() => void) | null = null;
let unsubscribeHistory: (() => void) | null = null;

export function initSavedQueriesSync(): void {
  unsubscribeSaved?.();
  unsubscribeSaved = control.onSavedQueriesChanged(({ connectionId }) => {
    void queryClient.invalidateQueries({ queryKey: ['filterHistory', connectionId] });
    void queryClient.invalidateQueries({ queryKey: ['consoleSaved', connectionId] });
    void queryClient.invalidateQueries({ queryKey: ['kibanaSaved', connectionId] });
  });
  unsubscribeHistory?.();
  unsubscribeHistory = control.onFilterHistoryChanged(({ connectionId, path }) => {
    void queryClient.invalidateQueries({ queryKey: ['filterHistory', connectionId, path] });
  });
}
