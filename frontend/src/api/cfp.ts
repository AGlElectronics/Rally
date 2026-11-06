import { apiRequest } from '@/api/client';
import type { Answers, Form, SubmissionState } from '@/lib/forms';
import type { UploadedFile } from '@/lib/upload';

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

export type ReviewMode = 'lead' | 'scored';
export type CfPStatus = 'draft' | 'open' | 'closed';

export type CallForPapers = {
  id: string;
  organization_id: string;
  edition_id: string;
  title: string;
  description: string;
  status: CfPStatus;
  opens_at?: string | null;
  closes_at?: string | null;
  form: Form;
  review_mode: ReviewMode;
  event_type_ids: string[];
  status_labels: Record<string, string>;
  created_at: string;
  updated_at: string;
};

export type CfPInput = {
  title: string;
  description: string;
  status: CfPStatus;
  opens_at?: string | null;
  closes_at?: string | null;
  form?: Form;
  review_mode?: ReviewMode;
  event_type_ids?: string[];
  status_labels?: Record<string, string>;
};

export type EventType = {
  id: string;
  edition_id: string;
  name: string;
  color: string;
  default_minutes: number;
  gap_after_minutes: number;
  sort_order: number;
  /** Rooms this type may use; empty = any room. */
  room_ids: string[];
};

export type EventTypeInput = Pick<EventType, 'name' | 'color' | 'default_minutes' | 'gap_after_minutes' | 'sort_order'> & {
  room_ids?: string[];
};

export type Person = {
  id: string;
  user_id?: string | null;
  display_name: string;
  role: 'owner' | 'presenter';
  status: 'invited' | 'accepted';
  invite_email?: string;
  invite_code?: string;
  created_at: string;
};

export type Submission = {
  id: string;
  edition_id: string;
  cfp_id: string;
  cfp_title: string;
  edition_title: string;
  submitter_user_id: string;
  submitter_name: string;
  title: string;
  answers: Answers;
  event_type_id?: string | null;
  event_type_name: string;
  duration_minutes?: number | null;
  state: SubmissionState;
  submitted_at?: string | null;
  decided_at?: string | null;
  decision_note: string;
  people: Person[];
  files: UploadedFile[];
  status_labels: Record<string, string>;
  cfp_accepting: boolean;
  score_avg?: number;
  score_count?: number;
  created_at: string;
  updated_at: string;
};

export type Note = { id: string; author_id?: string | null; author_name: string; body: string; created_at: string };
export type Score = { reviewer_id: string; reviewer_name: string; score: number; note: string; updated_at: string };
export type ScoreSummary = { review_mode: ReviewMode; average?: number | null; scores: Score[]; mine?: Score | null };

type List<T> = { items: T[]; total?: number };

export const API = '/api/v1';

export const listEditions = () => apiRequest<List<Edition>>(`${API}/editions`);

export const listCfPs = (editionId: string) => apiRequest<List<CallForPapers>>(`${API}/editions/${editionId}/cfps`);
export const getCfP = (id: string) => apiRequest<CallForPapers>(`${API}/cfps/${id}`);
export const createCfP = (editionId: string, body: CfPInput) =>
  apiRequest<CallForPapers>(`${API}/editions/${editionId}/cfps`, { method: 'POST', body });
export const updateCfP = (id: string, body: CfPInput) =>
  apiRequest<CallForPapers>(`${API}/cfps/${id}`, { method: 'PUT', body });

export const listEventTypes = (editionId: string) =>
  apiRequest<List<EventType>>(`${API}/editions/${editionId}/event-types`);
export const createEventType = (editionId: string, body: EventTypeInput) =>
  apiRequest<EventType>(`${API}/editions/${editionId}/event-types`, { method: 'POST', body });
export const updateEventType = (id: string, body: EventTypeInput) =>
  apiRequest<EventType>(`${API}/event-types/${id}`, { method: 'PUT', body });
export const deleteEventType = (id: string) => apiRequest<void>(`${API}/event-types/${id}`, { method: 'DELETE' });

export const listSubmissions = (editionId: string, query?: { cfp_id?: string; state?: string }) =>
  apiRequest<List<Submission>>(`${API}/editions/${editionId}/submissions`, { query });
export const getSubmission = (id: string) => apiRequest<Submission>(`${API}/submissions/${id}`);
export const acceptSubmission = (id: string, note: string) =>
  apiRequest<Submission>(`${API}/submissions/${id}/accept`, { method: 'POST', body: { note } });
export const rejectSubmission = (id: string, note: string) =>
  apiRequest<Submission>(`${API}/submissions/${id}/reject`, { method: 'POST', body: { note } });

export const listNotes = (id: string) => apiRequest<List<Note>>(`${API}/submissions/${id}/notes`);
export const addNote = (id: string, body: string) =>
  apiRequest<Note>(`${API}/submissions/${id}/notes`, { method: 'POST', body: { body } });
export const getScores = (id: string) => apiRequest<ScoreSummary>(`${API}/submissions/${id}/scores`);
export const setScore = (id: string, score: number, note: string) =>
  apiRequest<ScoreSummary>(`${API}/submissions/${id}/scores`, { method: 'PUT', body: { score, note } });
