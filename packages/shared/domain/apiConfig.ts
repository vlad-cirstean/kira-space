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
