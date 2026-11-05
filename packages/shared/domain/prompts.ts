import { z } from 'zod';

// P246: one popup the Go prompt router routes to a window (internal/prompts.Routed). Metadata only:
// the owning store holds the content and the answer. `target` "" means queued until a window opens.
export const promptKindSchema = /*#__PURE__*/ z.enum([
  'schedule',
  'git-credential',
  'mobile-pairing',
  'update',
  'quit',
  'handover',
  'proof',
  'approval',
]);
export type PromptKind = z.infer<typeof promptKindSchema>;

export const routedPromptSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  kind: promptKindSchema,
  ref: z.string(),
  origin: z.string(),
  title: z.string(),
  createdAt: z.number(),
  target: z.string(),
});
export type RoutedPrompt = z.infer<typeof routedPromptSchema>;

// P279: the live work a quit would stop (internal/shell.QuitStatus).
export interface QuitStatus {
  asking: boolean;
  agents: number;
  runs: number;
  scripts: number;
}
