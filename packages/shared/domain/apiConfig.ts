// Mirrors internal/apifiles View: the Api module's config files and the secret names they reference.
// Never holds a secret value; SetSecret is write-only.

export interface ApiFileProblem {
  /** 0 when the problem has no single line (a file over the size cap). */
  line: number;
  message: string;
}

export interface ApiConfigFile {
  file: string;
  kind: 'collection' | 'environment';
  path: string;
  name: string;
  problem: ApiFileProblem | null;
  /** The rows still follow the last valid text of this file. */
  lastGood: boolean;
}

export interface ApiSecret {
  name: string;
  set: boolean;
  referenced: boolean;
}

export interface ApiConfigView {
  dir: string;
  files: ApiConfigFile[];
  secrets: ApiSecret[];
  storage: {
    available: boolean;
    backend: string;
    insecureFallback: boolean;
    reason: string | null;
  };
}

/** Mirrors apifiles.FileDetail: one collection or environment file as the variables dialog edits it. */
export interface ApiFileDetail {
  file: string;
  kind: 'collection' | 'environment';
  path: string;
  text: string;
  hash: string;
  problem: ApiFileProblem | null;
  lastGood: boolean;
}

/** One variable in a file: a value or a `${secret:name}` reference, never both. */
export interface ApiFileVariable {
  name: string;
  value?: string;
  secret?: string;
  description?: string;
}

export interface ApiEnvironmentFileModel {
  name: string;
  description?: string;
  color?: string;
  variables?: ApiFileVariable[];
}

/** A collection file as the dialog sees it: only `variables` is edited; items stay in the file. */
export interface ApiCollectionFileModel {
  name: string;
  variables?: ApiFileVariable[];
  items?: unknown[];
}

/** The reply of ApiConfigService.Form: the edited text and what it decodes to. */
export interface ApiFormResult {
  text: string;
  value: ApiEnvironmentFileModel | ApiCollectionFileModel | null;
  problem: { line: number; col?: number; path?: string; message: string } | null;
}
