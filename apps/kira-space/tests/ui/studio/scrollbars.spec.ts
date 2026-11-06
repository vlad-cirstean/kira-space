import { registerScrollbarScenarios } from '@workbench/testing/ui/scenarios/scrollbars';
import { test } from './fixtures';

// P391: thumbs hidden until the pointer is inside the scroller (base.css). Pure CSS, no IPC.

registerScrollbarScenarios({ test });
