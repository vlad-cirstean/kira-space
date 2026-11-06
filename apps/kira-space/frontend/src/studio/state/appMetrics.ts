import { createAppMetricsStore } from '@workbench/state/createAppMetricsStore';
import { control } from '../bridge/control';

// P116 H6: body hoisted to createAppMetricsStore.ts, unchanged — Kira Space's own status-bar item
// (G7) shares it verbatim.
export const useAppMetricsStore = createAppMetricsStore(control);
