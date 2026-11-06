import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { CalendarDays, Download } from 'lucide-react';
import { ApiError, errorText } from '@/api/client';
import { buttonClasses } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Pill, selectClasses } from '@/components/ui/Pill';
import { EmptyState, ErrorRow, LoadingRow } from '@/components/ui/States';
import { cn } from '@/lib/utils';
import { dayLabel, timeIn } from '@/lib/zonedTime';
import { PUBLIC_API, getPublicSchedule, listSchedules } from '@/submit/api';

const notPublic = (e: unknown) => e instanceof ApiError && e.status === 404;

export function ProgrammePage() {
  const list = useQuery({ queryKey: ['public-schedules'], queryFn: listSchedules, retry: false });
  const editions = list.data?.items ?? [];
  const [picked, setPicked] = useState('');
  const editionId = editions.some((e) => e.edition_id === picked) ? picked : editions[0]?.edition_id;
  const schedule = useQuery({
    queryKey: ['public-schedule', editionId],
    queryFn: () => getPublicSchedule(editionId!),
    enabled: !!editionId,
    refetchInterval: 60_000,
  });
  const [day, setDay] = useState('');
  const [room, setRoom] = useState('');

  if (list.isLoading || schedule.isLoading) return <LoadingRow />;
  if (notPublic(list.error) || (!list.error && editions.length === 0)) {
    return <EmptyState icon={CalendarDays} title="No programme yet" description="The schedule appears here once the organisers publish it." />;
  }
  if (list.error || schedule.error) return <ErrorRow message={errorText(list.error || schedule.error)} />;

  const s = schedule.data!;
  const tz = s.edition.timezone;
  const current = s.days.includes(day) ? day : s.days[0];
  const items = s.items.filter((it) => it.day === current && (!room || it.room_id === room));

  return (
    <div>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="font-heading text-3xl font-extrabold tracking-tight">Programme</h1>
          <p className="mt-1 text-muted-foreground">
            {s.edition.title} · times in {tz}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {editions.length > 1 ? (
            <select aria-label="Event" className={selectClasses} value={editionId} onChange={(e) => setPicked(e.target.value)}>
              {editions.map((e) => (
                <option key={e.edition_id} value={e.edition_id}>
                  {e.title}
                </option>
              ))}
            </select>
          ) : null}
          <a className={buttonClasses({ variant: 'secondary', size: 'sm' })} href={`${PUBLIC_API}/schedules/${s.edition.id}/calendar.ics`}>
            <Download className="h-4 w-4" />
            Add to calendar
          </a>
        </div>
      </div>

      <div className="mt-6 flex flex-wrap items-center gap-2">
        {s.days.map((d) => (
          <button
            key={d}
            type="button"
            onClick={() => setDay(d)}
            className={cn(
              'rounded-pill px-3 py-1.5 text-sm font-semibold',
              d === current ? 'bg-primary text-primary-foreground' : 'bg-secondary text-secondary-foreground hover:bg-accent'
            )}
          >
            {dayLabel(d)}
          </button>
        ))}
        {s.rooms.length > 1 ? (
          <select aria-label="Room" className={cn(selectClasses, 'ml-auto')} value={room} onChange={(e) => setRoom(e.target.value)}>
            <option value="">All rooms</option>
            {s.rooms.map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </select>
        ) : null}
      </div>

      {items.length === 0 ? (
        <EmptyState className="mt-4" icon={CalendarDays} title="Nothing on this day" />
      ) : (
        <Card className="mt-4 divide-y divide-border">
          {items.map((it) => (
            <div key={it.id} className={cn('flex gap-4 px-4 py-3', it.status === 'cancelled' && 'opacity-60')}>
              <div className="w-20 shrink-0 tabular-nums">
                <p className="font-semibold">{timeIn(it.starts_at, tz)}</p>
                <p className="text-xs text-muted-foreground">to {timeIn(it.ends_at, tz)}</p>
              </div>
              <span className="w-1 shrink-0 rounded-full" style={{ backgroundColor: it.color || 'hsl(var(--muted-foreground))' }} />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <p className={cn('font-semibold', it.status === 'cancelled' && 'line-through')}>{it.title}</p>
                  {it.status === 'cancelled' ? <Pill tone="danger">Cancelled</Pill> : null}
                </div>
                <p className="text-sm text-muted-foreground">
                  {it.room}
                  {it.event_type ? ` · ${it.event_type}` : ''}
                  {it.people.length > 0 ? ` · ${it.people.join(', ')}` : ''}
                </p>
                {it.description ? <p className="mt-1 whitespace-pre-line text-sm">{it.description}</p> : null}
              </div>
            </div>
          ))}
        </Card>
      )}
    </div>
  );
}
