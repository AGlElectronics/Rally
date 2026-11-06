import { ApiError, apiRequest } from '@/api/client';
import type { Edition, EventType } from '@/api/cfp';

const API = '/api/v1';

export type RuleName = 'no_overlap' | 'gap' | 'person' | 'room_fit';
export type RuleMode = 'block' | 'warn' | 'off';

export const RULES: { id: RuleName; label: string; hint: string }[] = [
  { id: 'no_overlap', label: 'No overlap in a room', hint: 'Two items can’t share a room at the same time.' },
  { id: 'gap', label: 'Changeover gap', hint: 'After an event type with a gap, the room stays blocked that long.' },
  { id: 'person', label: 'Person in two places', hint: 'Someone presenting can’t be in two items at once (gaps included).' },
  { id: 'room_fit', label: 'Event type fits the room', hint: 'Event types limited to some rooms can only go there.' },
];

export type Room = {
  id: string;
  edition_id: string;
  name: string;
  capacity: number | null;
  sort_order: number;
};

export type RoomInput = { name: string; capacity: number | null; sort_order: number };

export type ScheduleSettings = {
  day_start_minute: number;
  day_end_minute: number;
  snap_minutes: number;
  rules: Record<RuleName, RuleMode>;
};

export type SchedulePerson = { user_id: string; name: string };

export type ScheduleItem = {
  id: string;
  submission_id: string | null;
  room_id: string;
  event_type_id: string | null;
  title: string;
  description: string;
  starts_at: string;
  ends_at: string;
  staff_only: boolean;
  status: 'scheduled' | 'cancelled';
  people: SchedulePerson[];
  updated_at: string;
};

export type Unscheduled = {
  id: string;
  title: string;
  cfp_title: string;
  event_type_id: string | null;
  duration_minutes: number;
  people: SchedulePerson[];
};

export type Conflict = {
  rule: RuleName;
  severity: 'block' | 'warn';
  item_id: string;
  other_id?: string;
  message: string;
};

export type PublishedInfo = { version: number; kind: string; note: string; published_at: string };

export type Schedule = {
  edition: Edition;
  settings: ScheduleSettings;
  rooms: Room[];
  event_types: EventType[];
  items: ScheduleItem[];
  unscheduled: Unscheduled[];
  conflicts: Conflict[];
  published: PublishedInfo | null;
  draft_changed: boolean;
  public_enabled: boolean;
  telegram_configured: boolean;
};

export type ItemInput = {
  submission_id?: string;
  title?: string;
  description?: string;
  event_type_id?: string | null;
  room_id?: string;
  starts_at?: string;
  ends_at?: string;
  duration_minutes?: number;
  staff_only?: boolean;
};

export type VersionSummary = {
  version: number;
  kind: 'publish' | 'live' | 'restore';
  note: string;
  published_at: string;
  published_by: string;
  item_count: number;
  is_current: boolean;
};

export type Version = VersionSummary & {
  data: {
    edition: { id: string; title: string; timezone: string };
    rooms: { id: string; name: string }[];
    event_types: { id: string; name: string; color: string }[];
    items: ScheduleItem[];
  };
};

export type LiveAction = 'delay' | 'cancel' | 'restore' | 'move';
export type LiveInput = {
  action: LiveAction;
  minutes?: number;
  push_room?: boolean;
  room_id?: string;
  starts_at?: string;
  force?: boolean;
  announce?: boolean;
};

export type Announcement = {
  id: string;
  edition_id: string;
  item_id: string | null;
  body: string;
  status: 'draft' | 'sent' | 'discarded';
  result: { chat: string; ok: boolean; dry_run?: boolean; error?: string }[];
  created_by: string;
  created_at: string;
  sent_by: string;
  sent_at: string | null;
};

export type LiveResult = {
  schedule: Schedule;
  version: number;
  moved: number;
  conflicts: Conflict[];
  announcement: Announcement | null;
};

export type TelegramChat = { id: string; edition_id: string; name: string; chat_id: string; enabled: boolean };
export type TelegramChatInput = Pick<TelegramChat, 'name' | 'chat_id' | 'enabled'>;

type List<T> = { items: T[] };

/** Conflicts attached to a refused change (409). */
export function conflictsOf(err: unknown): Conflict[] {
  if (err instanceof ApiError && err.details && typeof err.details === 'object') {
    const c = (err.details as { conflicts?: Conflict[] }).conflicts;
    if (Array.isArray(c)) return c;
  }
  return [];
}

export const updateEditionDates = (editionId: string, body: { timezone: string; start_at: string | null; end_at: string | null }) =>
  apiRequest<Edition>(`${API}/editions/${editionId}/dates`, { method: 'PUT', body });

export const listRooms = (editionId: string) => apiRequest<List<Room>>(`${API}/editions/${editionId}/rooms`);
export const createRoom = (editionId: string, body: RoomInput) =>
  apiRequest<Room>(`${API}/editions/${editionId}/rooms`, { method: 'POST', body });
export const updateRoom = (id: string, body: RoomInput) => apiRequest<Room>(`${API}/rooms/${id}`, { method: 'PUT', body });
export const deleteRoom = (id: string) => apiRequest<void>(`${API}/rooms/${id}`, { method: 'DELETE' });

export const getSchedule = (editionId: string) => apiRequest<Schedule>(`${API}/editions/${editionId}/schedule`);
export const updateScheduleSettings = (editionId: string, body: ScheduleSettings) =>
  apiRequest<Schedule>(`${API}/editions/${editionId}/schedule/settings`, { method: 'PUT', body });
export const createItem = (editionId: string, body: ItemInput) =>
  apiRequest<Schedule>(`${API}/editions/${editionId}/schedule/items`, { method: 'POST', body });
export const updateItem = (id: string, body: ItemInput) =>
  apiRequest<Schedule>(`${API}/schedule-items/${id}`, { method: 'PATCH', body });
export const deleteItem = (id: string) => apiRequest<Schedule>(`${API}/schedule-items/${id}`, { method: 'DELETE' });

export const publishSchedule = (editionId: string, note: string) =>
  apiRequest<Schedule>(`${API}/editions/${editionId}/schedule/publish`, { method: 'POST', body: { note } });
export const listVersions = (editionId: string) =>
  apiRequest<List<VersionSummary>>(`${API}/editions/${editionId}/schedule/versions`);
export const getVersion = (editionId: string, version: number) =>
  apiRequest<Version>(`${API}/editions/${editionId}/schedule/versions/${version}`);
export const restoreVersion = (editionId: string, version: number) =>
  apiRequest<{ schedule: Schedule; skipped: string[] }>(`${API}/editions/${editionId}/schedule/versions/${version}/restore`, {
    method: 'POST',
  });

export const liveChange = (editionId: string, itemId: string, body: LiveInput) =>
  apiRequest<LiveResult>(`${API}/editions/${editionId}/schedule/live/${itemId}`, { method: 'POST', body });

export const listAnnouncements = (editionId: string) =>
  apiRequest<List<Announcement>>(`${API}/editions/${editionId}/announcements`);
export const createAnnouncement = (editionId: string, body: string) =>
  apiRequest<Announcement>(`${API}/editions/${editionId}/announcements`, { method: 'POST', body: { body } });
export const updateAnnouncement = (id: string, body: string) =>
  apiRequest<Announcement>(`${API}/announcements/${id}`, { method: 'PUT', body: { body } });
export const sendAnnouncement = (id: string) => apiRequest<Announcement>(`${API}/announcements/${id}/send`, { method: 'POST' });
export const discardAnnouncement = (id: string) =>
  apiRequest<Announcement>(`${API}/announcements/${id}/discard`, { method: 'POST' });

// Keys for the public schedule API. The value comes back once on create;
// the backend stores only its hash. The builtin "programme-page" key is
// rotated by changing PUBLIC_API_BUILTIN_KEY, never through this API.
export type PublicApiKey = {
  id: string;
  name: string;
  prefix: string;
  builtin: boolean;
  created_by: string;
  created_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
};

export const listApiKeys = () => apiRequest<List<PublicApiKey>>(`${API}/public-api-keys/`);
export const createApiKey = (name: string) =>
  apiRequest<{ key: PublicApiKey; token: string }>(`${API}/public-api-keys/`, { method: 'POST', body: { name } });
export const revokeApiKey = (id: string) => apiRequest<void>(`${API}/public-api-keys/${id}`, { method: 'DELETE' });

export const listChats = (editionId: string) => apiRequest<List<TelegramChat>>(`${API}/editions/${editionId}/telegram-chats`);
export const createChat = (editionId: string, body: TelegramChatInput) =>
  apiRequest<TelegramChat>(`${API}/editions/${editionId}/telegram-chats`, { method: 'POST', body });
export const updateChat = (id: string, body: TelegramChatInput) =>
  apiRequest<TelegramChat>(`${API}/telegram-chats/${id}`, { method: 'PUT', body });
export const deleteChat = (id: string) => apiRequest<void>(`${API}/telegram-chats/${id}`, { method: 'DELETE' });
