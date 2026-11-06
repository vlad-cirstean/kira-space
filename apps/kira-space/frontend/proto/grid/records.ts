import { GUTTER_FIELD } from './data';
import { stageValue } from './pending';
import type { ProtoState } from './state';

/** One page row as the grid's field lookup sees it. Fields are prototype getters, so a record
 *  costs one object and a cell is decoded only when drawn. */
export interface RowRecord {
  readonly row: number;
  /** Display position, which the gutter numbers. */
  readonly position: number;
}

export function createRecordClass(
  state: ProtoState,
): new (
  row: number,
  position: number,
) => RowRecord {
  class Record implements RowRecord {
    constructor(
      readonly row: number,
      readonly position: number,
    ) {}
  }
  const proto = Record.prototype;
  state.data.columns.forEach((column, index) => {
    Object.defineProperty(proto, column.name, {
      get(this: Record) {
        return state.viewAt(this.row, index);
      },
      // The grid's editor, paste and range delete write through `record[field] = value`.
      set(this: Record, value: string) {
        stageValue(state, this.row, index, value);
      },
    });
  });
  Object.defineProperty(proto, GUTTER_FIELD, {
    get(this: Record) {
      return String(state.rowNumberBase + this.position + 1);
    },
  });
  return Record;
}
