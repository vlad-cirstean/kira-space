// Work colours: `Task.color` is an index into this 20-slot table, assigned server-side.
const PALETTE = [
  '#d38261',
  '#d2a155',
  '#b4b04d',
  '#90ba66',
  '#64b477',
  '#57b1a0',
  '#56a6c2',
  '#5a8fd5',
  '#727ed7',
  '#9676d1',
  '#bc72cb',
  '#d36fa4',
  '#d56c7e',
  '#b18765',
  '#95a269',
  '#669d8e',
  '#8092b0',
  '#a182b7',
  '#cc9191',
  '#a4aab2',
] as const;

export function taskColor(slot: number): string {
  return PALETTE[((slot % PALETTE.length) + PALETTE.length) % PALETTE.length] as string;
}
