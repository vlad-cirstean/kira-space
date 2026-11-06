import { wrapSelectionOnType } from '@theme/wrapSelection';
import type {
  Column,
  Editor,
  EditorArguments,
  EditorValidationResult,
  GridOption,
} from 'slickgrid';
import type { RowHandle } from './dataSource';

// The editor is a one-row `<textarea>`: a text `<input>` strips line breaks on paste and on
// `value =`, so a multi-line value would silently flatten and re-stage on leaving the cell.
// Typing a break stays blocked (see `blockModifiedEnter`); breaks come from a paste or the value.
//
// The incumbent's own single overlay `<input>` (
// `editingCellRect`/`editingCell`/`editingBuffer`/`isEditing`/`.cell-input-overlay`,
// `RowSig.editingCol`) goes away entirely. SlickGrid puts the editor *inside* the active cell node
// itself (`makeActiveCellEditable`, `slick.grid.ts:4004-4075`: `args.container` IS
// `this.activeCellNode`, emptied first unless `suppressClearOnEdit`).
//
// A grid-owned instance cannot be constructed with tabId/displayCell/stageEdit closed over (the
// grid calls `new useEditor(editorArgs)` itself, `slick.grid.ts:4059`, passing only
// `EditorArguments`) so SlickGridHost.vue binds this file's own `editorCtx` — the same reassigned-
// plain-object idiom `formatterCtx` already uses there for the identical reason (nothing on
// this synchronous edit-open/commit path may create a Vue reactive dependency).
export interface EditorContext {
  /** The cell's current, staged-aware display text for `(page row, column name)`. */
  readValue: (row: number, columnName: string) => { text: string; isNull: boolean };
  /** Stages the serialized buffer verbatim. Never writes to the grid's own `item`/page — see
   *  `applyValue`'s own comment for why that would throw. */
  commit: (row: number, columnName: string, value: string) => void;
}

export const editorCtx: EditorContext = {
  readValue: () => ({ text: '', isNull: true }),
  commit: () => {},
};

function blockModifiedEnter(e: KeyboardEvent): void {
  if (e.key === 'Enter' && (e.shiftKey || e.altKey)) e.preventDefault();
}

// Matches slick.editors.ts's own stock editors' generic defaults (Column<T>'s `field` can't be
// widened otherwise — see KiraColumn's own identical comment in SlickGridHost.vue).
// biome-ignore lint/suspicious/noExplicitAny: see comment above.
type AnyData = any;

/** Confirmed against source, not assumed: `slick.grid.ts` never calls `.init()` on a
 *  constructed editor (a `grep -n "currentEditor.init"` over the whole file returns nothing) — the
 *  documented `Editor.init(args)` entry point is a convention other editors use, not a contract
 *  SlickGrid enforces, so this class does its DOM-building work in the constructor itself rather
 *  than depending on an external call that never comes. `init` is still implemented, as a no-op, to
 *  satisfy the `Editor` interface for anything that *does* call it (a future composite editor,
 *  `editor.interface.ts`'s own doc comment on that path).
 */
export class KiraCellEditor<
  TData = AnyData,
  C extends Column<TData> = Column<TData>,
  O extends GridOption<C> = GridOption<C>,
> implements Editor
{
  private readonly args: EditorArguments<TData, C, O>;
  private readonly input: HTMLTextAreaElement;
  private loaded = '';

  constructor(args: EditorArguments<TData, C, O>) {
    this.args = args;
    this.input = document.createElement('textarea');
    this.input.rows = 1;
    this.input.className = 'cell-input';
    this.input.dataset.testid = 'grid-cell-input';
    // The input is a descendant of the grid's own keydown-handling container (`enableCellNavigation`
    // owns Enter-to-commit/Escape-to-cancel for free — see grid options' own comment — so this
    // listener only needs the wrap-on-type behaviour, not a full onEditKeydown port).
    this.input.addEventListener('keydown', wrapSelectionOnType);
    this.input.addEventListener('keydown', blockModifiedEnter);
    args.container.appendChild(this.input);
    this.input.focus();
    this.input.select();
  }

  init(): void {
    // See this class's own file-level comment: SlickGrid never calls this. No-op, present only to
    // satisfy the `Editor` interface.
  }

  loadValue(item: RowHandle): void {
    const name = String(this.args.column.field);
    const dc = editorCtx.readValue(item.row, name);
    this.loaded = dc.isNull ? '' : dc.text;
    this.input.value = this.loaded;
    this.input.focus();
    this.input.select();
  }

  serializeValue(): string {
    return this.input.value;
  }

  /** The rule: never write to `item`. `RowHandle` is `Object.freeze`d (`dataSource.ts`) and the
   *  page behind it is frozen with a tripwire (`store.ts`) — a write would throw, which is the
   *  *correct* behaviour here, not a bug to route around. `this.args.item` (the row this editor was
   *  opened for, captured at construction) is used instead of the `item` parameter SlickGrid
   *  passes, since a `updateRow`/`updateItem` between open and commit could otherwise hand back a
   *  different reference than the one this editor is editing. */
  applyValue(_item: unknown, state: string): void {
    const name = String(this.args.column.field);
    const row = (this.args.item as RowHandle).row;
    editorCtx.commit(row, name, state);
  }

  isValueChanged(): boolean {
    return this.input.value !== this.loaded;
  }

  // No client-side validation in this phase (D8) — the server is the source of truth for
  // constraints, and this editor only ever expresses text (P24 D14's own NULL scope limit: a
  // retype, never a "set to NULL" affordance — Set NULL stays the cell menu's own item).
  validate(): EditorValidationResult {
    return { valid: true, msg: null };
  }

  focus(): void {
    this.input.focus();
  }

  destroy(): void {
    this.input.removeEventListener('keydown', wrapSelectionOnType);
    this.input.removeEventListener('keydown', blockModifiedEnter);
    this.input.remove();
  }
}
