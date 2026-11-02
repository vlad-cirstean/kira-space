// Mirrors internal/agentcfg View, SaveArgs, SetSecretArgs and DeleteSecretArgs.
export interface AgentSettingsView {
  folder: string;
  file: string;
  path: string;
  exists: boolean;
  text: string;
  hash: string /* identifies text; Save sends it back as baseHash */;
  editable: boolean;
  problem: { line: number; message: string } | null;
  lastGood: boolean;
  maxBytes: number;
  storage: {
    available: boolean;
    backend: string;
    insecureFallback: boolean;
    reason: string | null;
  };
  secrets: AgentSecretView[];
  providers: AgentProviderView[];
}
export interface AgentSecretView {
  name: string;
  set: boolean;
  referenced: boolean;
}
export interface AgentProviderView {
  id: string;
  label: string;
  files: { name: string; text: string }[];
  env: { name: string; value: string }[];
  notes: string[];
  kira: string[];
}
export interface AgentSettingsSaveArgs {
  text: string;
  baseHash: string;
}
export interface AgentSetSecretArgs {
  name: string;
  value: string;
}
