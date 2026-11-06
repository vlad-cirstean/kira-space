import type { ConnectionKind } from '@shared/domain/connection';
import type { BeautifyMode, BeautifyResult } from '../../beautify';
import type { CopyAllFormat } from '../../views/console/copyAll';
import type { FormatResult } from '../../views/console/format';

export type PrettyFormat = 'json' | 'xml';

export interface PrettyResult {
  format: PrettyFormat | null;
  text?: string;
}

interface ParseJobs {
  'body.format': { input: { body: string; wantText: boolean }; output: PrettyResult };
  'json.beautify': { input: { text: string; mode: BeautifyMode }; output: BeautifyResult };
  'console.format': { input: { kind: ConnectionKind; text: string }; output: FormatResult };
  'ejson.copyAll': { input: { format: CopyAllFormat; bodies: readonly string[] }; output: string };
  'xml.beautify': { input: { text: string; mode: BeautifyMode }; output: BeautifyResult };
}

export type JobKind = keyof ParseJobs;
export type JobInput<K extends JobKind> = ParseJobs[K]['input'];
export type JobOutput<K extends JobKind> = ParseJobs[K]['output'];

export interface WorkerRequest {
  id: number;
  kind: JobKind;
  input: unknown;
}

export type WorkerResponse =
  | { id: number; ok: true; output: unknown }
  | { id: number; ok: false; error: string };
