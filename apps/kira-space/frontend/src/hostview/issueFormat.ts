/** Up to two capital letters of a person's name, for the avatar circle (no remote avatar can load). */
export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  const letters = parts.length > 1 ? [parts[0], parts[parts.length - 1]] : parts;
  return letters.map((p) => p?.charAt(0).toUpperCase()).join('') || '?';
}

/** The status badge tone of a status category. */
export function statusVariant(category: string | undefined): 'ok' | 'info' | 'chip' {
  switch (category) {
    case 'done':
      return 'ok';
    case 'indeterminate':
      return 'info';
    default:
      return 'chip';
  }
}
