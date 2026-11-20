import { z } from 'zod';
import { scriptRunStateSchema } from './scriptRuns';

// P212: the Mobile access pane's domain. bridge/mobile.go never carries a token or hash across
// this boundary.

export const mobileDeviceSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  label: z.string(),
  userAgent: z.string(),
  createdAt: z.number(),
  lastSeenAt: z.number(),
  lastIp: z.string(),
  revokedAt: z.number().nullable(),
  /** Epoch ms after which the phone must pair again. */
  expiresAt: z.number(),
  /** Change the backlog and move or start tasks. Set from the desktop pane. */
  canWrite: z.boolean(),
  /** Reply to agents and control their terminals. Needs the global switch too. */
  canAgentInput: z.boolean(),
});
export type MobileDevice = z.infer<typeof mobileDeviceSchema>;

export const mobileNetworkSchema = /*#__PURE__*/ z.object({
  interface: z.string(),
  /** This computer's address on the network; empty for the stored trusted network. */
  address: z.string(),
  subnet: z.string(),
  routerIp: z.string(),
  routerMac: z.string(),
});
export type MobileNetwork = z.infer<typeof mobileNetworkSchema>;

/** Why an enabled server is stopped; '' while it runs or is disabled. */
export const mobileStopReasonSchema = /*#__PURE__*/ z.enum([
  '',
  'notTrusted',
  'away',
  'otherRouter',
  'unavailable',
]);

export const mobileStatusSchema = /*#__PURE__*/ z.object({
  enabled: z.boolean(),
  running: z.boolean(),
  port: z.number(),
  appUrl: z.string(),
  /** Global switch: phones may reply to agents and control their terminals. */
  agentInput: z.boolean(),
  error: z.string(),
  stopReason: mobileStopReasonSchema,
  stopDetail: z.string(),
  /** What the default route shows now; null when it cannot be read. */
  current: mobileNetworkSchema.nullable(),
  trusted: mobileNetworkSchema.nullable(),
  /** Epoch ms; 0 when nothing is trusted. */
  trustedAt: z.number(),
});
export type MobileStatus = z.infer<typeof mobileStatusSchema>;

export const mobilePairingRequestSchema = /*#__PURE__*/ z.object({
  requestId: z.string(),
  clientId: z.string(),
  /** Phone-reported, not verified. */
  label: z.string(),
  /** Four digits the phone shows too, to tell this request from another device's. */
  code: z.string(),
  remoteIp: z.string(),
  userAgent: z.string(),
  expiresAtMs: z.number(),
});
export type MobilePairingRequest = z.infer<typeof mobilePairingRequestSchema>;

export const mobilePairingSnapshotSchema = /*#__PURE__*/ z.object({
  pending: mobilePairingRequestSchema.nullable(),
  queued: z.number(),
});
export type MobilePairingSnapshot = z.infer<typeof mobilePairingSnapshotSchema>;

export const mobilePairingActionResultSchema = /*#__PURE__*/ z.object({
  result: /*#__PURE__*/ z.enum(['resolved', 'alreadyResolved', 'expired']),
});
export type MobilePairingActionResult = z.infer<typeof mobilePairingActionResultSchema>;

export const mobileTerminalHoldSchema = /*#__PURE__*/ z.object({
  terminalId: z.string(),
  sessionId: z.string(),
  deviceId: z.string(),
  label: z.string(),
  connected: z.boolean(),
  since: z.number(),
  /** When an offline hold falls back to the desktop; 0 while connected. */
  returnsAt: z.number(),
});
export type MobileTerminalHold = z.infer<typeof mobileTerminalHoldSchema>;

// What a phone reads from /api/runs: a keep-list projection of scriptruns.Run. Never a cwd,
// command, prompt, params, terminal id or output (internal/mobileweb/scriptruns.go).
export const phoneRunSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  scriptName: z.string(),
  color: z.string(),
  kind: z.enum(['script', 'smart']),
  trigger: z.enum(['terminal', 'manual', 'ade', 'scheduled', 'mcp']),
  state: scriptRunStateSchema,
  createdAt: z.number(),
  startedAt: z.number().nullable(),
  finishedAt: z.number().nullable(),
  taskId: z.string(),
  taskTitle: z.string(),
  branchId: z.string(),
  branchLabel: z.string(),
  resumable: z.boolean(),
  outcome: /*#__PURE__*/ z
    .object({ status: z.string(), reason: z.string(), exitCode: z.number().optional() })
    .nullable(),
});
export type PhoneRun = z.infer<typeof phoneRunSchema>;

export const phoneRunsSchema = /*#__PURE__*/ z.object({ runs: z.array(phoneRunSchema) });

// What a phone reads from /api/computer. `docker` is null while the Docker module is off.
export const phoneComputerSchema = /*#__PURE__*/ z.object({
  docker: /*#__PURE__*/ z
    .object({
      state: z.enum(['ok', 'unavailable']),
      reason: z.string(),
      running: z.number(),
      stopped: z.number(),
      total: z.number(),
    })
    .nullable(),
});
export type PhoneComputer = z.infer<typeof phoneComputerSchema>;
