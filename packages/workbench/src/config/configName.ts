/** Config file name without its extension, for rows and titles that name a file by identity. */
export function configName(file: string): string {
  return file.replace(/\.(yaml|yml|md)$/, '');
}
