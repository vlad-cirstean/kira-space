/** JSON text re-indented, or null when it does not parse or re-indenting would change a value (a big
 *  integer, an escape): the caller then shows the text as it is. */
export function prettyJson(text: string): string | null {
  try {
    const pretty = JSON.stringify(JSON.parse(text), null, 2);
    return pretty.replace(/\s/g, '') === text.replace(/\s/g, '') ? pretty : null;
  } catch {
    return null;
  }
}
