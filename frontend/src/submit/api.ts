import { apiRequest } from '@/api/client';
import type { Submission } from '@/api/cfp';
import type { AnswerValue, Form } from '@/lib/forms';
import { runtimeConfig } from '@/runtime-config';

export type { Person, Submission } from '@/api/cfp';

export const PUBLIC_API = '/api/public/v1';

export type PublicEventType = { id: string; name: string; color: string; default_minutes: number };

export type PublicCall = {
  id: string;
  edition_id: string;
  edition_title: string;
  edition_timezone: string;
  title: string;
  description: string;
  opens_at?: string | null;
  closes_at?: string | null;
  accepting_now: boolean;
  form: Form;
  event_types: PublicEventType[];
  status_labels: Record<string, string>;
};

export type Profile = {
  id: string;
  display_name: string;
  email: string;
  email_verified: boolean;
  presenter_code: string;
};

export type InvitePreview = {
  code: string;
  submission_id: string;
  title: string;
  cfp_title: string;
  edition_title: string;
  invited_by: string;
  already_on_it: boolean;
  submission_open: boolean;
};

export type SubmissionBody = {
  title?: string;
  answers?: Record<string, AnswerValue | undefined>;
  event_type_id?: string | null;
  duration_minutes?: number | null;
};

export const listCalls = () => apiRequest<{ items: PublicCall[] }>(`${PUBLIC_API}/calls`);
export const getCall = (id: string) => apiRequest<PublicCall>(`${PUBLIC_API}/calls/${id}`);
export const getProfile = () => apiRequest<Profile>(`${PUBLIC_API}/me`);

export const listMine = () => apiRequest<{ items: Submission[] }>(`${PUBLIC_API}/submissions`);
export const getMine = (id: string) => apiRequest<Submission>(`${PUBLIC_API}/submissions/${id}`);
export const createMine = (cfpId: string, body: SubmissionBody) =>
  apiRequest<Submission>(`${PUBLIC_API}/calls/${cfpId}/submissions`, { method: 'POST', body });
export const updateMine = (id: string, body: SubmissionBody) =>
  apiRequest<Submission>(`${PUBLIC_API}/submissions/${id}`, { method: 'PATCH', body });
export const submitMine = (id: string) => apiRequest<Submission>(`${PUBLIC_API}/submissions/${id}/submit`, { method: 'POST' });
export const withdrawMine = (id: string) =>
  apiRequest<Submission>(`${PUBLIC_API}/submissions/${id}/withdraw`, { method: 'POST' });
export const leaveSubmission = (id: string) =>
  apiRequest<void>(`${PUBLIC_API}/submissions/${id}/leave`, { method: 'POST' });

export const addPerson = (id: string, body: { email?: string; presenter_code?: string }) =>
  apiRequest<Submission>(`${PUBLIC_API}/submissions/${id}/people`, { method: 'POST', body });
export const removePerson = (id: string, personId: string) =>
  apiRequest<Submission>(`${PUBLIC_API}/submissions/${id}/people/${personId}`, { method: 'DELETE' });
export const reissueInvite = (id: string, personId: string) =>
  apiRequest<Submission>(`${PUBLIC_API}/submissions/${id}/people/${personId}/reissue`, { method: 'POST' });

export const previewInvite = (code: string) => apiRequest<InvitePreview>(`${PUBLIC_API}/invites/${encodeURIComponent(code)}`);
export const acceptInvite = (code: string) =>
  apiRequest<Submission>(`${PUBLIC_API}/invites/${encodeURIComponent(code)}/accept`, { method: 'POST' });
export const declineInvite = (code: string) =>
  apiRequest<void>(`${PUBLIC_API}/invites/${encodeURIComponent(code)}/decline`, { method: 'POST' });

export type PublicScheduleListing = { edition_id: string; slug: string; title: string; version: number; published_at: string };

export type PublicScheduleItem = {
  id: string;
  title: string;
  description: string;
  room_id: string;
  room: string;
  event_type_id: string | null;
  event_type: string;
  color: string;
  starts_at: string;
  ends_at: string;
  day: string;
  status: 'scheduled' | 'cancelled';
  people: string[];
};

export type PublicSchedule = {
  edition: { id: string; slug: string; title: string; timezone: string };
  version: number;
  published_at: string;
  days: string[];
  rooms: { id: string; name: string }[];
  items: PublicScheduleItem[];
};

// The public schedule API requires an API key; the Programme page uses the
// builtin "programme-page" one (publicScheduleKey in /config.json, paired with the
// backend's PUBLIC_API_BUILTIN_KEY). It ships with this bundle, so it buys
// rotation and per-client tracking, not secrecy.
const scheduleHeaders = () => {
  const key = runtimeConfig().publicScheduleKey;
  return key ? { Authorization: `Bearer ${key}` } : undefined;
};

export const listSchedules = () =>
  apiRequest<{ items: PublicScheduleListing[] }>(`${PUBLIC_API}/schedules`, { headers: scheduleHeaders() });
export const getPublicSchedule = (edition: string) =>
  apiRequest<PublicSchedule>(`${PUBLIC_API}/schedules/${edition}`, { headers: scheduleHeaders() });
