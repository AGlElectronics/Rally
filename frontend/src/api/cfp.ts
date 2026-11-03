import { apiRequest } from '@/api/client';

export type Edition = {
  id: string;
  organization_id: string;
  convention_id: string;
  slug: string;
  title: string;
  timezone: string;
  status: string;
  start_at?: string | null;
  end_at?: string | null;
};

export type CallForPapers = {
  id: string;
  organization_id: string;
  edition_id: string;
  title: string;
  description: string;
  status: 'draft' | 'open' | 'closed';
  opens_at?: string | null;
  closes_at?: string | null;
};

export type Speaker = { name: string; email?: string; bio?: string };

export type Submission = {
  id: string;
  cfp_id: string;
  edition_id: string;
  title: string;
  abstract: string;
  speakers: Speaker[];
  state: 'draft' | 'submitted' | 'accepted' | 'rejected' | 'withdrawn';
  decision_note?: string;
};

export const listEditions = () =>
  apiRequest<{ items: Edition[]; total: number }>('/api/v1/editions');

export const listCfPs = (editionId: string) =>
  apiRequest<{ items: CallForPapers[]; total: number }>(`/api/v1/editions/${editionId}/cfps`);

export const createCfP = (
  editionId: string,
  body: { title: string; description?: string; status: string },
) => apiRequest<CallForPapers>(`/api/v1/editions/${editionId}/cfps`, { method: 'POST', body });

export const listSubmissions = (editionId: string, query?: { cfp_id?: string; mine?: boolean }) =>
  apiRequest<{ items: Submission[]; total: number }>(`/api/v1/editions/${editionId}/submissions`, {
    query: {
      cfp_id: query?.cfp_id,
      mine: query?.mine ? '1' : undefined,
    },
  });

export const createSubmission = (
  cfpId: string,
  body: { title: string; abstract: string; speakers: Speaker[] },
) => apiRequest<Submission>(`/api/v1/cfps/${cfpId}/submissions`, { method: 'POST', body });

export const submitSubmission = (id: string) =>
  apiRequest<Submission>(`/api/v1/submissions/${id}/submit`, { method: 'POST' });

export const acceptSubmission = (id: string, note?: string) =>
  apiRequest<Submission>(`/api/v1/submissions/${id}/accept`, {
    method: 'POST',
    body: { note: note ?? '' },
  });

export const rejectSubmission = (id: string, note?: string) =>
  apiRequest<Submission>(`/api/v1/submissions/${id}/reject`, {
    method: 'POST',
    body: { note: note ?? '' },
  });
