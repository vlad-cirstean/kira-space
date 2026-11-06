// The one control.filesChooseOpen wrapper both file-body surfaces share — form-data's
// file rows (FormDataTable.vue) and the binary body picker (BinaryBodyPicker.vue). Only
// {path, name, size} ever crosses back — never a byte of the file's own contents. Mirrors
// UploadObjectDialog.vue's own chooseFile (workbench/UploadObjectDialog.vue:42-49), the S3
// precedent this design is built on.
import { control } from '../../bridge/control';

export interface PickedFile {
  path: string;
  name: string;
  size: number;
}

export async function chooseBodyFile(title?: string): Promise<PickedFile | null> {
  const res = await control.filesChooseOpen(title ? { title } : undefined);
  if (res.canceled || !res.file) return null;
  return { path: res.file.path, name: res.file.name, size: res.file.size };
}
