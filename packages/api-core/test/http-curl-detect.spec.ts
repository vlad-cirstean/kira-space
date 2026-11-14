// looksLikeCurlCommand earns a dedicated test where the rest of the batch does not. It is
// a recogniser with several interacting rules (prompt prefixes, quoting, a path-qualified command
// name, a whole-token match, a required argument) guarding a destructive action: a false positive
// silently replaces the user's entire request with whatever parseCurl makes of text that was never
// a curl command. parseCurl itself is already covered by http-curl.spec.ts; the guard was not.

import { describe, expect, test } from 'bun:test';
import { looksLikeCurlCommand } from '../src/http/curl/detect';

describe('looksLikeCurlCommand', () => {
  test('a plain curl command', () => {
    expect(looksLikeCurlCommand('curl https://api.example.com/users')).toBe(true);
  });

  test('leading and trailing whitespace, and a leading blank line', () => {
    expect(looksLikeCurlCommand('\n\n   curl https://x.dev  ')).toBe(true);
  });

  test.each([['$ '], ['# '], ['> '], ['% '], ['❯ ']])(
    'a %p shell prompt is skipped',
    (prompt: string) => {
      expect(looksLikeCurlCommand(`${prompt}curl https://x.dev`)).toBe(true);
    },
  );

  test('a multi-line, backslash-continued command is decided by its first line', () => {
    const cmd = `curl 'https://api.example.com/o' \\\n  -H 'Accept: application/json' \\\n  --data '{"a":1}'`;
    expect(looksLikeCurlCommand(cmd)).toBe(true);
  });

  test('a path-qualified command name, posix and windows', () => {
    expect(looksLikeCurlCommand('/usr/bin/curl https://x.dev')).toBe(true);
    expect(looksLikeCurlCommand('C:\\tools\\curl.exe https://x.dev')).toBe(true);
  });

  test('a quoted command name', () => {
    expect(looksLikeCurlCommand('"C:\\Program Files\\curl.exe" https://x.dev')).toBe(true);
  });

  test('case-insensitive on the command name only', () => {
    expect(looksLikeCurlCommand('CURL https://x.dev')).toBe(true);
  });

  // --- everything below must paste literally, never replace the request ------------------------

  test('a bare URL that merely contains the word curl', () => {
    expect(looksLikeCurlCommand('https://api.example.com/curl')).toBe(false);
  });

  test('a token that only starts with curl', () => {
    expect(looksLikeCurlCommand('curl-config --version')).toBe(false);
    expect(looksLikeCurlCommand('wget https://h')).toBe(false);
    expect(looksLikeCurlCommand('curlopts https://x.dev')).toBe(false);
  });

  test('the bare word curl, with no argument, is not a request to blank the current one', () => {
    expect(looksLikeCurlCommand('curl')).toBe(false);
    expect(looksLikeCurlCommand('  curl  ')).toBe(false);
    expect(looksLikeCurlCommand('$ curl\n')).toBe(false);
  });

  test('curl appearing on a later line does not count — the first line decides', () => {
    expect(looksLikeCurlCommand('POST /users HTTP/1.1\ncurl https://x.dev')).toBe(false);
  });

  test('empty and whitespace-only input', () => {
    expect(looksLikeCurlCommand('')).toBe(false);
    expect(looksLikeCurlCommand('   \n\t\n')).toBe(false);
  });

  test('a JSON body pasted on its own', () => {
    expect(looksLikeCurlCommand('{"name":"gizmo","cmd":"curl x"}')).toBe(false);
  });
});
