import { toCodedError } from '@workbench/bridge/codedError';
import { confirmOverwrite } from './confirmOverwrite';

// Decision 3: a saved query another window changed answers `E_CONFLICT` with `details.changed`
// (a name collision is also `E_CONFLICT` but carries no such detail). The user confirms before
// the write is resent without its guard; declining returns undefined and writes nothing.
export async function guardedSavedWrite<T>(
  write: (guarded: boolean) => Promise<T>,
): Promise<T | undefined> {
  try {
    return await write(true);
  } catch (err) {
    const coded = toCodedError(err);
    const changed = (coded.details as { changed?: boolean } | undefined)?.changed === true;
    if (coded.code !== 'E_CONFLICT' || !changed) throw err;
    if (!(await confirmOverwrite(coded.message))) return undefined;
    return write(false);
  }
}
