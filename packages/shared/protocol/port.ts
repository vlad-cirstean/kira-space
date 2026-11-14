export interface PortRequest {
  kind: 'req';
  id: number;
  op: string;
  payload: unknown;
}

export type PortResponse =
  | { kind: 'res'; id: number; ok: true; payload: unknown }
  | {
      kind: 'res';
      id: number;
      ok: false;
      error: { message: string; code?: string; details?: string };
    };

export interface PortEvent {
  kind: 'evt';
  topic: string;
  payload: unknown;
}

export interface PingPayload {
  pong: true;
  enginePid: number;
  at: number;
}
