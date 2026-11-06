import { useQueryClient } from '@tanstack/react-query';
import type { EventType } from '@/api/cfp';
import type { Conflict, Schedule, ScheduleItem } from '@/api/schedule';
import { daysBetween, zonedParts } from '@/lib/zonedTime';

/** The days to plan: the edition's date range, plus any day items sit on. */
export function scheduleDays(sc: Schedule): string[] {
  const tz = sc.edition.timezone;
  const days = new Set<string>();
  if (sc.edition.start_at && sc.edition.end_at) {
    for (const d of daysBetween(sc.edition.start_at, sc.edition.end_at, tz)) days.add(d);
  }
  for (const it of sc.items) days.add(zonedParts(it.starts_at, tz).date);
  return [...days].sort();
}

export function byId<T extends { id: string }>(list: T[]): Map<string, T> {
  return new Map(list.map((x) => [x.id, x]));
}

/** Conflicts per item (both sides of a pair). */
export function conflictsByItem(cs: Conflict[]): Map<string, Conflict[]> {
  const m = new Map<string, Conflict[]>();
  const add = (id: string, c: Conflict) => m.set(id, [...(m.get(id) ?? []), c]);
  for (const c of cs) {
    add(c.item_id, c);
    if (c.other_id) add(c.other_id, c);
  }
  return m;
}

export function worstSeverity(cs: Conflict[] | undefined): 'block' | 'warn' | null {
  if (!cs || cs.length === 0) return null;
  return cs.some((c) => c.severity === 'block') ? 'block' : 'warn';
}

export function itemMinutes(it: Pick<ScheduleItem, 'starts_at' | 'ends_at'>): number {
  return Math.round((new Date(it.ends_at).getTime() - new Date(it.starts_at).getTime()) / 60000);
}

export function typeColor(types: Map<string, EventType>, id: string | null | undefined): string {
  return (id && types.get(id)?.color) || '#64748b';
}

/** Replaces the cached schedule with a server response. */
export function usePutSchedule(editionId: string) {
  const qc = useQueryClient();
  return (next: Schedule) => qc.setQueryData(['schedule', editionId], next);
}
