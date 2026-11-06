/** The tree needs one unique string key per row, and collections and items are separate id spaces. */
export function collectionKey(id: string): string {
  return `c:${id}`;
}

export function itemKey(id: string): string {
  return `i:${id}`;
}
