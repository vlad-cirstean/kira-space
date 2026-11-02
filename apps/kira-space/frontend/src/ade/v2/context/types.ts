// Mirrors internal/agentctx View and SaveArgs.
export interface AgentContextView {
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
  keepsOnResume: string[] /* product labels whose resumed conversation keeps its recorded context */;
}
export interface AgentContextSaveArgs {
  text: string;
  baseHash: string;
}
