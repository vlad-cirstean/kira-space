// The mode seam: one id per module. A tab's mode is a total function of its kind (TAB_KIND_MODE,
// tabs.ts), there is no mode column, and switching mode writes nothing.
export type AppMode =
  | 'ade'
  | 'automations'
  | 'memory'
  | 'git'
  | 'studio'
  | 'api'
  | 'docker'
  | 'kubernetes';
