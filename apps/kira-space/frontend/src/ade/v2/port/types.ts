// Mirrors internal/agentport wire types.
interface PortFile {
  role: string;
  path: string;
  exists: boolean;
}
export interface PortProvider {
  id: string;
  label: string;
  files: PortFile[] | null;
}
interface SecretSlot {
  name: string;
  field: string;
  reuse: boolean;
}
export interface PortItem {
  id: string;
  kind: string;
  source: string;
  state: 'new' | 'same' | 'conflict' | 'skipped';
  reason: string;
  modes: string[] | null;
  text: string;
  current: string;
  secrets: SecretSlot[] | null;
}
interface PortSource {
  role: string;
  path: string;
  exists: boolean;
  problem: string;
}
export interface ImportPreview {
  token: string;
  provider: string;
  sources: PortSource[] | null;
  items: PortItem[] | null;
  notes: string[] | null;
}
interface ImportSelection {
  id: string;
  mode: string;
}
export interface ImportApplyArgs {
  token: string;
  items: ImportSelection[] | null;
}
interface ExportFile {
  role: string;
  path: string;
  exists: boolean;
  changed: boolean;
  before: string;
  after: string;
  language: string;
}
interface ExportServer {
  name: string;
  action: string;
  transport: string;
  target: string;
}
interface PortSkipped {
  what: string;
  reason: string;
}
export interface ExportPreview {
  token: string;
  provider: string;
  files: ExportFile[] | null;
  servers: ExportServer[] | null;
  envNames: string[] | null;
  skipped: PortSkipped[] | null;
  notes: string[] | null;
  cliMissing: boolean;
}
export interface ExportApplyArgs {
  token: string;
  files: string[] | null;
  servers: boolean;
}
interface ServerResult {
  name: string;
  outcome: string;
  detail: string;
}
export interface PortResult {
  written: string[] | null;
  backups: string[] | null;
  servers: ServerResult[] | null;
  secrets: string[] | null;
  notes: string[] | null;
}
