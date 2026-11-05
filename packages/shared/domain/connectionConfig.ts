// Mirrors internal/connfiles View and FileDetail: the connection files' status, text and the secret
// names they reference. Never holds a secret value; SetSecret is write-only.

export interface ConnectionFileProblem {
  /** 0 when the problem has no single line (a file over the size cap). */
  line: number;
  message: string;
}

export interface ConnectionConfigFile {
  file: string;
  /** The connection's row id; empty for a file that was never valid. */
  id: string;
  name: string;
  problem: ConnectionFileProblem | null;
  /** The row still follows the last valid text of this file. */
  lastGood: boolean;
}

export type ConnectionSecretKind = 'password' | 'uri';

export interface ConnectionSecret {
  kind: ConnectionSecretKind;
  name: string;
  set: boolean;
  referenced: boolean;
}

export interface ConnectionConfigView {
  dir: string;
  files: ConnectionConfigFile[];
  secrets: ConnectionSecret[];
  storage: {
    available: boolean;
    backend: string;
    insecureFallback: boolean;
    reason: string | null;
  };
}

export interface ConnectionFileDetail {
  file: string;
  id: string;
  path: string;
  text: string;
  hash: string;
  problem: ConnectionFileProblem | null;
  lastGood: boolean;
  editable: boolean;
  /** The secrets this file's effective record names. */
  secrets: ConnectionSecret[];
}

/** A connection file in its own key vocabulary (internal/connfiles File). Passwords and URIs are
 *  `${secret:name}` references, never values. */
export interface ConnectionFileMask {
  table?: string;
  column: string;
  kind: string;
  keep_hint?: boolean;
  correlate?: boolean;
}

export interface ConnectionFileMcp {
  enabled?: boolean;
  description?: string;
  read?: string;
  write?: string;
  ddl?: string;
  auto_explain?: boolean;
  review_results?: string;
}

export interface ConnectionFileModel {
  name: string;
  kind: string;
  color?: string;
  read_only?: boolean;
  mode?: string;
  host?: string;
  port?: number;
  database?: string;
  username?: string;
  password?: string;
  uri?: string;
  options?: Record<string, unknown>;
  preconnect?: { command: string; sidecar?: boolean; delay_sec?: number };
  auto_explain?: boolean;
  throttle_per_sec?: number;
  mcp?: ConnectionFileMcp;
  masks?: ConnectionFileMask[];
}

/** The reply of ConnectionConfigService.Form: the edited text and what it decodes to. */
export interface ConnectionFormResult {
  text: string;
  value: ConnectionFileModel | null;
  problem: { line: number; col?: number; path?: string; message: string } | null;
}
