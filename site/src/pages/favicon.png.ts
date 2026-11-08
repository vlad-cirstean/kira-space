import { readFile } from 'node:fs/promises';
import sharp from 'sharp';

export async function GET() {
  const png = await sharp(await readFile(__APP_ICON__))
    .resize(64, 64)
    .png()
    .toBuffer();
  return new Response(new Uint8Array(png), { headers: { 'Content-Type': 'image/png' } });
}
