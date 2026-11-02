// Wire types of CaptureService (apps/kira-space/internal/bridge/capture.go) and shell.HotkeyState.

type CaptureKind = 'changeRequest' | 'issue' | 'text' | 'empty';

export interface CaptureItem {
  id: string;
  kind: CaptureKind;
  text: string;
  truncated: boolean;
  /** "PR #7 owner/repo" or "Jira PROJ-12"; '' for text and empty. */
  label: string;
  /** Prefill for the task title. */
  title: string;
}

export interface CaptureResult {
  taskId: string;
  /** What the added task lacks ('' when complete); the popup stays open to show it. */
  notice: string;
}

type HotkeyStateKind = 'off' | 'active' | 'requested' | 'failed';

export interface HotkeyState {
  state: HotkeyStateKind;
  accelerator: string;
  reason?: string;
}

/** Go-side push channels. */
export const CHANNEL_CAPTURE_ITEM = 'kira:capture:item';
export const CHANNEL_CAPTURE_STATUS = 'kira:capture:status';
