// The TypeScript mirrors of Go's model.Variable/model.Environment/model.VariableHistoryEntry
// and apivars.RevealResult. Following the rule — "the wire shapes live in Go and are
// mirrored, not re-validated" — these are types, not guards; control.ts `trust<T>()`s them like
// every other bound result. Nothing here becomes tab state (unlike httpSavedRequestSchema in
// collections.ts), so none of it earns the one Zod-parse boundary that file's own comment explains.
//
// A variable is resolved for both protocols (Api* names the module).
import type { PaletteColor } from './color';

export const VARIABLE_SCOPES = ['collection', 'environment'] as const;
export type VariableScope = (typeof VARIABLE_SCOPES)[number];

// The row model, deliberately without `enabled` or `type`.
// `value` is '' whenever `isSecret` — a secret's plaintext never crosses the
// bridge except through the gated Reveal. `description` is a plain field, not a secret.
export interface ApiVariable {
  id: string;
  scope: VariableScope;
  ownerId: string;
  name: string;
  value: string;
  isSecret: boolean;
  sortOrder: number;
  description: string;
}

// The active environment is app-global, a column on the row itself rather than a layout leaf
// or a settings key. `description` is app-local free text, no Postman round-trip question at all
// (unlike a variable's). `color` is a user-assigned value from the same palette Studio's
// connections use (PaletteColor), never derived; 'none' is the default.
export interface ApiEnvironment {
  id: string;
  name: string;
  /** The environment's config file name in the api folder. */
  file: string;
  sortOrder: number;
  isActive: boolean;
  description: string;
  color: PaletteColor;
}

// D13: history is per-entry — an environment has no history of its own, only its entries do.
export interface ApiVariableHistoryEntry {
  id: string;
  variableId: string;
  value: string;
  isSecret: boolean;
  recordedAt: string;
}

// D8: the same four-outcome vocabulary v1.1 P14 already established for connections.RevealResult,
// redeclared here rather than shared — importing internal/connections from internal/apivars would
// be exactly the Studio<->Api coupling this app's module-boundary rule exists to prevent.
export type RevealOutcome = 'revealed' | 'cancelled' | 'confirmation-required' | 'error';

export interface RevealResult {
  value: string | null;
  error: string | null;
  outcome: RevealOutcome;
}
