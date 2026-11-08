#!/usr/bin/env bun
/** Fails when the committed share image no longer matches its inputs (headline, icon, font). */
import { readFileSync } from 'node:fs';
import { HASH_FILE, shareImageHash } from './share-image-input';

const recorded = readFileSync(HASH_FILE, 'utf8').trim();
if (recorded !== shareImageHash()) {
  process.stderr.write('check-share-image: share image is stale, run bun run site:share-image\n');
  process.exit(1);
}
process.stderr.write('check-share-image: share image matches its inputs\n');
