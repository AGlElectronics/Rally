import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { ExternalLink, Trash2, X } from 'lucide-react';
import { errorText } from '@/api/client';
import { createItem, deleteItem, updateItem, type Conflict, type Schedule, type ScheduleItem } from '@/api/schedule';
import { itemMinutes } from '@/components/schedule/shared';
import { textareaClasses } from '@/components/forms/FormFields';
import { Button } from '@/components/ui/Button';
import { Input, Label } from '@/components/ui/Input';
import { selectClasses } from '@/components/ui/Pill';
import { cn } from '@/lib/utils';
import { dayLabel, minutesLabel, parseClock, zonedParts, zonedToUtc } from '@/lib/zonedTime';

export type PanelTarget =
  | { mode: 'new'; day: string; roomId?: string; start?: number }
  | { mode: 'edit'; item: ScheduleItem };

type Draft = {
  title: string;
  description: string;
  eventTypeId: string;
  roomId: string;
  day: string;
  start: string;
  minutes: number;
  staffOnly: boolean;
};

function initial(sc: Schedule, t: PanelTarget): Draft {
  const tz = sc.edition.timezone;
  if (t.mode === 'edit') {
    const p = zonedParts(t.item.starts_at, tz);
    return {
      title: t.item.title,
      description: t.item.description,
      eventTypeId: t.item.event_type_id ?? '',
      roomId: t.item.room_id,
      day: p.date,
      start: minutesLabel(p.minutes),
      minutes: itemMinutes(t.item),
      staffOnly: t.item.staff_only,
    };
  }
  return {
    title: '',
    description: '',
    eventTypeId: '',
    roomId: t.roomId ?? sc.rooms[0]?.id ?? '',
    day: t.day,
    start: minutesLabel(t.start ?? sc.settings.day_start_minute),
    minutes: 30,
    staffOnly: false,
  };
}

export function ItemPanel({
  schedule: sc,
  target,
  days,
  canEdit,
  conflicts,
  onClose,
  onSaved,
}: {
  schedule: Schedule;
  target: PanelTarget;
  days: string[];
  canEdit: boolean;
  conflicts: Conflict[];
  onClose: () => void;
  onSaved: (sc: Schedule) => void;
}) {
  const tz = sc.edition.timezone;
  const [d, setD] = useState<Draft>(() => initial(sc, target));
  const set = (p: Partial<Draft>) => setD((x) => ({ ...x, ...p }));
  const item = target.mode === 'edit' ? target.item : null;

  const save = useMutation({
    mutationFn: () => {
      const start = parseClock(d.start);
      if (start === null) throw new Error('Start time must look like 14:30');
      const body = {
        title: d.title,
        description: d.description,
        event_type_id: d.eventTypeId || null,
        room_id: d.roomId,
        starts_at: zonedToUtc(d.day, start, tz),
        duration_minutes: d.minutes,
        staff_only: d.staffOnly,
      };
      return item ? updateItem(item.id, body) : createItem(sc.edition.id, body);
    },
    onSuccess: onSaved,
  });
  const remove = useMutation({ mutationFn: () => deleteItem(item!.id), onSuccess: onSaved });

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/30" onClick={onClose}>
      <div
        className="flex h-full w-full max-w-md flex-col overflow-y-auto border-l border-border bg-card shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-5 py-4">
          <h2 className="font-heading text-lg font-bold">{item ? 'Schedule item' : 'Add an item'}</h2>
          <button type="button" aria-label="Close" className="rounded-pill p-1.5 hover:bg-secondary" onClick={onClose}>
            <X className="h-5 w-5" />
          </button>
        </div>
        <form
          className="flex-1 space-y-4 px-5 py-4"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          {conflicts.length > 0 ? (
            <ul className="space-y-1 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs">
              {conflicts.map((c, i) => (
                <li key={i} className={c.severity === 'block' ? 'text-destructive' : ''}>
                  <span className="font-semibold">{c.severity === 'block' ? 'Blocks' : 'Warning'}:</span> {c.message}
                </li>
              ))}
            </ul>
          ) : null}
          {item?.submission_id ? (
            <Link
              to="/submissions/$submissionId"
              params={{ submissionId: item.submission_id }}
              className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
            >
              <ExternalLink className="h-3.5 w-3.5" />
              Open the submission
            </Link>
          ) : null}
          <div>
            <Label htmlFor="it-title">Title</Label>
            <Input id="it-title" disabled={!canEdit} value={d.title} onChange={(e) => set({ title: e.target.value })} required />
          </div>
          <div>
            <Label htmlFor="it-desc">Public description</Label>
            <textarea
              id="it-desc"
              disabled={!canEdit}
              className={cn(textareaClasses, 'min-h-24')}
              value={d.description}
              onChange={(e) => set({ description: e.target.value })}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label htmlFor="it-type">Event type</Label>
              <select
                id="it-type"
                disabled={!canEdit}
                className={cn(selectClasses, 'w-full')}
                value={d.eventTypeId}
                onChange={(e) => {
                  const t = sc.event_types.find((x) => x.id === e.target.value);
                  set({ eventTypeId: e.target.value, ...(!item && t ? { minutes: t.default_minutes } : {}) });
                }}
              >
                <option value="">None</option>
                {sc.event_types.map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.name}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <Label htmlFor="it-room">Room</Label>
              <select
                id="it-room"
                disabled={!canEdit}
                className={cn(selectClasses, 'w-full')}
                value={d.roomId}
                onChange={(e) => set({ roomId: e.target.value })}
              >
                {sc.rooms.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.name}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <Label htmlFor="it-day">Day</Label>
              <select
                id="it-day"
                disabled={!canEdit}
                className={cn(selectClasses, 'w-full')}
                value={d.day}
                onChange={(e) => set({ day: e.target.value })}
              >
                {days.map((day) => (
                  <option key={day} value={day}>
                    {dayLabel(day)}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <Label htmlFor="it-start">Starts ({tz})</Label>
              <Input
                id="it-start"
                type="time"
                step={300}
                disabled={!canEdit}
                value={d.start}
                onChange={(e) => set({ start: e.target.value })}
              />
            </div>
            <div>
              <Label htmlFor="it-len">Length (min)</Label>
              <Input
                id="it-len"
                type="number"
                min={5}
                step={5}
                disabled={!canEdit}
                value={d.minutes}
                onChange={(e) => set({ minutes: Number(e.target.value) || 0 })}
              />
            </div>
            <label className="flex items-end gap-2 pb-2 text-sm">
              <input
                type="checkbox"
                disabled={!canEdit}
                checked={d.staffOnly}
                onChange={(e) => set({ staffOnly: e.target.checked })}
                className="h-4 w-4 accent-primary"
              />
              Staff only (not public)
            </label>
          </div>
          {item && item.people.length > 0 ? (
            <div>
              <Label>People</Label>
              <p className="text-sm">{item.people.map((p) => p.name).join(', ')}</p>
            </div>
          ) : null}
          {item?.status === 'cancelled' ? (
            <p className="text-sm text-warning">Cancelled on the live day. Restore it from the Live day tab.</p>
          ) : null}
          {save.error || remove.error ? <p className="text-sm text-destructive">{errorText(save.error || remove.error)}</p> : null}
          {canEdit ? (
            <div className="flex items-center gap-2 pt-2">
              <Button type="submit" disabled={save.isPending}>
                {item ? 'Save' : 'Add to schedule'}
              </Button>
              <Button type="button" variant="secondary" onClick={onClose}>
                Cancel
              </Button>
              {item ? (
                <Button
                  type="button"
                  variant="ghost"
                  className="ml-auto text-destructive"
                  disabled={remove.isPending}
                  onClick={() => remove.mutate()}
                >
                  <Trash2 className="h-4 w-4" />
                  {item.submission_id ? 'Unschedule' : 'Delete'}
                </Button>
              ) : null}
            </div>
          ) : null}
        </form>
      </div>
    </div>
  );
}
