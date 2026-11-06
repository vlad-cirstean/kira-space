import { base64ToBytes as decodeBase64 } from '@shared/domain/base64';
import { BASE64_STD_RE, BASE64_URL_RE, base64ToStd } from './detect';

/** `null` means "not decodable as text" — an invalid encoding, or bytes that decode fine but
 *  aren't valid UTF-8 (arbitrary binary data). The caller (CellEditorView.vue) shows a note
 *  instead of a second editor in that case, rather than rendering garbled bytes as if they were
 *  meaningful text. */
export function decodeToText(format: 'hex' | 'base64', text: string): string | null {
  const t = text.trim();
  if (t.length === 0) return '';
  const bytes = format === 'hex' ? hexToBytes(t) : base64ToBytes(t);
  if (!bytes) return null;
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes);
  } catch {
    return null;
  }
}

function hexToBytes(t: string): Uint8Array | null {
  const digits = t.startsWith('0x') || t.startsWith('0X') ? t.slice(2) : t;
  if (digits.length === 0) return new Uint8Array(0);
  if (digits.length % 2 !== 0 || /[^0-9a-fA-F]/.test(digits)) return null;
  const bytes = new Uint8Array(digits.length / 2);
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = Number.parseInt(digits.slice(i * 2, i * 2 + 2), 16);
  }
  return bytes;
}

function base64ToBytes(t: string): Uint8Array | null {
  const isUrlSafe = !BASE64_STD_RE.test(t) && BASE64_URL_RE.test(t);
  if (!BASE64_STD_RE.test(t) && !isUrlSafe) return null;
  try {
    return decodeBase64(base64ToStd(t, isUrlSafe));
  } catch {
    return null;
  }
}

/** Re-encodes edited plaintext back into the cell's encoding. `previousEncoded` is consulted only
 *  to preserve a `0x` hex prefix if the value being replaced had one — every other stylistic
 *  choice (case, padding) is the encoding's own canonical form, since there is no "original style"
 *  to preserve for bytes that no longer exist once the plaintext changed underneath them. */
export function encodeFromText(
  format: 'hex' | 'base64',
  text: string,
  previousEncoded: string,
): string {
  const bytes = new TextEncoder().encode(text);
  if (format === 'hex') {
    const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
    const hadPrefix = /^0x/i.test(previousEncoded.trim());
    return hadPrefix ? `0x${hex}` : hex;
  }
  let bin = '';
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin);
}
