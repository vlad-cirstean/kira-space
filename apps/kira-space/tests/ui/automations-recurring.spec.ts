import { registerRecurringScenarios } from '@workbench/testing/ui/scenarios/automationsRecurring';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P242 Part 4: recurring scripts on the mocked bridge: rows, the target dialog, the confirm popup.

registerRecurringScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [{ channel: IPC.windowsEnsure, response: { mode: 'git' } }] },
  ({ BASE, SCRIPT, openPanel }) => {
    test('contract: the target dialog targets an ADE task through the task picker', async ({
      relaunch,
    }) => {
      const adePreview = contract<{ needs: { tasks: { id: string; title: string }[] } }>(
        'schedule-task-picker',
        'ScriptRunsService.Preview#list-tasks',
      );
      const sent = contract<{ taskId: string; branchId: string }>(
        'schedule-task-picker',
        'args:CustomScriptsService.SetScheduleTarget#task',
      );
      const picked = adePreview.needs.tasks[0];
      expect(picked?.id).toBe(sent.taskId);
      const { window: page, control } = await relaunch({
        control: [
          ...BASE,
          { channel: IPC.scriptRunsPreview, response: adePreview },
          { channel: IPC.customScriptsSetScheduleTarget, response: SCRIPT },
        ],
      });
      await openPanel(page);
      await page.locator(`[data-testid="script-${SCRIPT.id}"]`).click({ button: 'right' });
      await page.locator('[data-testid="menu-item-edit-target"]').click();
      const dialog = page.locator('[data-testid="schedule-target-dialog"]');
      await dialog.locator('[data-testid="run-task"]').click();
      await page.locator(`[data-testid="run-task-option-${picked?.id}"]`).click();
      await dialog.locator('[data-testid="schedule-target-save"]').click();
      await expect
        .poll(
          () => control.log().find((e) => e.channel === IPC.customScriptsSetScheduleTarget)?.args,
        )
        .toMatchObject({ id: SCRIPT.id, taskId: sent.taskId });
    });
  },
);
