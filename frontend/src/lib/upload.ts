import { apiRequest } from '@/api/client';

export type UploadedFile = {
  id: string;
  field_id: string;
  filename: string;
  content_type: string;
  size_bytes: number;
};

type Ticket = { file_id: string; url: string; fields: Record<string, string>; expires_at: string };

/**
 * Uploads a file for a form field. The backend only hands out a presigned
 * POST; the bytes go straight from the browser to object storage, which
 * enforces the size and type limits itself.
 */
export async function uploadFile(apiBase: string, cfpId: string, fieldId: string, file: File): Promise<UploadedFile> {
  const ticket = await apiRequest<Ticket>(`${apiBase}/uploads`, {
    method: 'POST',
    body: {
      cfp_id: cfpId,
      field_id: fieldId,
      filename: file.name,
      content_type: file.type || 'application/octet-stream',
      size_bytes: file.size,
    },
  });
  const body = new FormData();
  for (const [k, v] of Object.entries(ticket.fields)) body.append(k, v);
  body.append('file', file);
  const res = await fetch(ticket.url, { method: 'POST', body });
  if (!res.ok) {
    throw new Error(
      res.status === 400 || res.status === 403
        ? 'Storage refused the file (too large or not an allowed type).'
        : `Upload failed (${res.status}).`,
    );
  }
  return apiRequest<UploadedFile>(`${apiBase}/uploads/${ticket.file_id}/complete`, { method: 'POST' });
}

/** Opens a short-lived download link for a file. */
export async function openFile(apiBase: string, fileId: string): Promise<void> {
  const { url } = await apiRequest<{ url: string }>(`${apiBase}/files/${fileId}/download`);
  window.open(url, '_blank', 'noopener');
}
