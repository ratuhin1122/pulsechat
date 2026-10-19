export const acceptedFiles = '.jpg,.jpeg,.png,.gif,.webp,.pdf,.zip,.docx,.xlsx,.pptx,.mp3,.wav,.ogg,.opus,.webm';

export function fileKind(file: File): 'image' | 'document' | 'voice' {
  const extension = file.name.split('.').pop()?.toLowerCase() ?? '';
  if (['jpg', 'jpeg', 'png', 'gif', 'webp'].includes(extension)) return 'image';
  if (['pdf', 'zip', 'docx', 'xlsx', 'pptx'].includes(extension)) return 'document';
  if (['mp3', 'wav', 'ogg', 'opus', 'webm'].includes(extension)) return 'voice';
  throw new Error('This file extension is not supported.');
}

export function validateFile(file: File): void {
  const kind = fileKind(file);
  const max = { image: 15, document: 25, voice: 10 }[kind];
  if (file.size < 100) throw new Error('Files must contain at least 100 bytes.');
  if (file.size > max * 1024 * 1024) throw new Error(`${kind} files must be ${max} MB or smaller.`);
}

export function fileSize(bytes: number): string {
  return bytes < 1024 * 1024 ? `${Math.max(1, Math.round(bytes / 1024))} KB` : `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export function duration(seconds: number): string {
  const value = Number.isFinite(seconds) ? Math.max(0, Math.floor(seconds)) : 0;
  return `${Math.floor(value / 60)}:${String(value % 60).padStart(2, '0')}`;
}

// GIFs retain their animation. Compression is opt-in and falls back to the original.
export async function compressImage(file: File): Promise<File> {
  if (!/\.(jpe?g|png|webp)$/i.test(file.name) || typeof createImageBitmap !== 'function') return file;
  let bitmap: ImageBitmap | undefined;
  try {
    bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' });
    if (bitmap.width * bitmap.height > 50_000_000) return file;
    const scale = Math.min(1, 1600 / Math.max(bitmap.width, bitmap.height));
    const canvas = document.createElement('canvas');
    canvas.width = Math.max(1, Math.round(bitmap.width * scale));
    canvas.height = Math.max(1, Math.round(bitmap.height * scale));
    const context = canvas.getContext('2d');
    if (!context) return file;
    context.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
    const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/webp', 0.85));
    return blob && blob.type === 'image/webp' && blob.size >= 100 && blob.size < file.size
      ? new File([blob], file.name.replace(/\.[^.]+$/, '.webp'), { type: 'image/webp' }) : file;
  } catch { return file; }
  finally { bitmap?.close(); }
}
