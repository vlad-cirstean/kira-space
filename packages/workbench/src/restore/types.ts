/** A Compose service inside a project row. */
export interface RestoreMember {
  id: string;
  label: string;
  error?: string;
}

/** A second choice on a row, on only while the row itself is chosen. */
export interface RestoreRowOption {
  label: string;
  /** Always on: shown checked and disabled. */
  required?: boolean;
  /** Why it is off for this row: shown disabled, the reason as a tooltip. */
  unsupportedReason?: string;
  /** On when the row first shows. */
  default?: boolean;
}

/**
 * One thing the shared restore dialog offers to start again. `ready` rows are selectable,
 * `waiting` rows start on their own and stay disabled, `unavailable` rows are listed with their
 * `reason` and never selected.
 */
export interface RestoreRow {
  id: string;
  label: string;
  /** Second line: a command or an image. */
  detail?: string;
  folder?: string;
  badges?: string[];
  state: 'ready' | 'waiting' | 'unavailable';
  /** The source's own status; rendered as `data-status` only. */
  status: string;
  reason?: string;
  error?: string;
  members?: RestoreMember[];
  /** Heading the row sits under; rows with the same group sit together, in row order. */
  group?: string;
  /** Visible label of the row's own checkbox, when the option needs it named. */
  toggleLabel?: string;
  option?: RestoreRowOption;
}

/** What the dialog confirms: the chosen row ids and, of those, the ones with the option on. */
export interface RestoreConfirm {
  ids: string[];
  options: string[];
}
