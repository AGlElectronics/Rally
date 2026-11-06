import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, Ban, Clock, Megaphone, MoveRight, Radio, RotateCcw, Send, Trash2 } from 'lucide-react';
import { errorText } from '@/api/client';
import {
  conflictsOf,
  createAnnouncement,
  discardAnnouncement,
  getVersion,
  listAnnouncements,
  liveChange,
  sendAnnouncement,
  updateAnnouncement,
  type Announcement,
  type Conflict,
  type LiveInput,
  type Schedule,
  type ScheduleItem,
} from '@/api/schedule';
import { textareaClasses } from '@/components/forms/FormFields';
import { byId, itemMinutes, typeColor, usePutSchedule } from '@/components/schedule/shared';
import { Button } from '@/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { Pill, selectClasses } from '@/components/ui/Pill';
import { EmptyState, LoadingRow } from '@/components/ui/States';
import { PERMS, useMe } from '@/hooks/useMe';
import { cn } from '@/lib/utils';
import { dayLabel, minutesLabel, parseClock, timeIn, zonedParts, zonedToUtc } from '@/lib/zonedTime';

type Blocked = { itemId: string; input: LiveInput; text: string; conflicts: Conflict[] };

export function LiveTab({ schedule: sc, onSetup }: { schedule: Schedule; onSetup: () => void }) {
  const canAct = useMe().can(PERMS.schedulePublish);
  const tz = sc.edition.timezone;
  const qc = useQueryClient();
  const put = usePutSchedule(sc.edition.id);
  const version = sc.published?.version ?? 0;
  const published = useQuery({
    queryKey: ['schedule-version', sc.edition.id, version],
    queryFn: () => getVersion(sc.edition.id, version),
    enabled: version > 0,
  });
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(t);
  }, []);

  const items = published.data?.data.items ?? [];
  const rooms = byId(published.data?.data.rooms ?? []);
  const types = byId(sc.event_types);
  const days = useMemo(() => [...new Set(items.map((it) => zonedParts(it.starts_at, tz).date))].sort(), [items, tz]);
  const today = zonedParts(new Date(now).toISOString(), tz).date;
  const [day, setDay] = useState('');
  const current = days.includes(day) ? day : days.includes(today) ? today : days[0];
  const [roomFilter, setRoomFilter] = useState('');
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [blocked, setBlocked] = useState<Blocked | null>(null);

  const act = useMutation({
    mutationFn: ({ itemId, input }: { itemId: string; input: LiveInput }) => liveChange(sc.edition.id, itemId, input),
    onSuccess: (res) => {
      put(res.schedule);
      void qc.invalidateQueries({ queryKey: ['schedule-version', sc.edition.id] });
      void qc.invalidateQueries({ queryKey: ['announcements', sc.edition.id] });
      setBlocked(null);
      const warn = res.conflicts.length > 0 ? ` ${res.conflicts.length} rule warning(s): ${res.conflicts[0].message}` : '';
      setMessage({
        ok: true,
        text: `Live now as version ${res.version}${res.moved > 1 ? ` (${res.moved} items moved)` : ''}.${res.announcement ? ' An announcement is waiting for you to confirm.' : ''}${warn}`,
      });
    },
    onError: (e, vars) => {
      const cs = conflictsOf(e);
      if (cs.length > 0) {
        setBlocked({ itemId: vars.itemId, input: vars.input, text: errorText(e), conflicts: cs });
        setMessage(null);
      } else {
        setMessage({ ok: false, text: errorText(e) });
      }
    },
  });
  const run = (itemId: string, input: LiveInput) => act.mutate({ itemId, input });

  if (!sc.published) {
    return (
      <EmptyState
        icon={Radio}
        title="Nothing published yet"
        description="Live-day changes apply to the published schedule. Build it and publish first."
      />
    );
  }

  const dayItems = items
    .filter((it) => zonedParts(it.starts_at, tz).date === current && (!roomFilter || it.room_id === roomFilter))
    .sort((a, b) => a.starts_at.localeCompare(b.starts_at) || (rooms.get(a.room_id)?.name ?? '').localeCompare(rooms.get(b.room_id)?.name ?? ''));

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_24rem]">
      <div className="space-y-3">
        <Card className="flex flex-wrap items-center gap-2 px-4 py-3">
          <Radio className="h-4 w-4 text-success" />
          <span className="text-sm">
            Changes here go out at once as a new published version
            {sc.public_enabled ? '' : ' (the public API is switched off)'}, and draft an announcement for Telegram.
          </span>
          <div className="ml-auto flex flex-wrap gap-2">
            <select aria-label="Day" className={selectClasses} value={current} onChange={(e) => setDay(e.target.value)}>
              {days.map((d) => (
                <option key={d} value={d}>
                  {dayLabel(d)}
                  {d === today ? ' (today)' : ''}
                </option>
              ))}
            </select>
            <select aria-label="Room" className={selectClasses} value={roomFilter} onChange={(e) => setRoomFilter(e.target.value)}>
              <option value="">All rooms</option>
              {[...rooms.values()].map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </select>
          </div>
        </Card>

        {message ? (
          <p className={cn('rounded-lg px-4 py-2 text-sm', message.ok ? 'bg-success/10 text-success' : 'bg-destructive/10 text-destructive')}>
            {message.text}
          </p>
        ) : null}
        {blocked ? (
          <div className="space-y-2 rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm">
            <p className="flex items-center gap-2 font-semibold">
              <AlertTriangle className="h-4 w-4 text-destructive" />
              {blocked.text}
            </p>
            {blocked.conflicts.length > 1 ? (
              <ul className="list-disc pl-6 text-xs text-muted-foreground">
                {blocked.conflicts.map((c, i) => (
                  <li key={i}>{c.message}</li>
                ))}
              </ul>
            ) : null}
            <div className="flex gap-2">
              <Button size="sm" variant="destructive" onClick={() => run(blocked.itemId, { ...blocked.input, force: true })}>
                Apply anyway
              </Button>
              <Button size="sm" variant="secondary" onClick={() => setBlocked(null)}>
                Don’t change
              </Button>
            </div>
          </div>
        ) : null}

        {published.isLoading ? (
          <LoadingRow />
        ) : dayItems.length === 0 ? (
          <EmptyState icon={Clock} title="Nothing on this day" description="Pick another day or room." />
        ) : (
          <Card className="divide-y divide-border">
            {dayItems.map((it) => (
              <LiveRow
                key={it.id}
                item={it}
                tz={tz}
                day={current}
                now={now}
                room={rooms.get(it.room_id)?.name ?? '?'}
                rooms={[...rooms.values()]}
                color={typeColor(types, it.event_type_id)}
                canAct={canAct}
                busy={act.isPending}
                onAct={(input) => run(it.id, input)}
              />
            ))}
          </Card>
        )}
      </div>
      <Announcements schedule={sc} canAct={canAct} onSetup={onSetup} />
    </div>
  );
}

function LiveRow({
  item,
  tz,
  day,
  now,
  room,
  rooms,
  color,
  canAct,
  busy,
  onAct,
}: {
  item: ScheduleItem;
  tz: string;
  day: string;
  now: number;
  room: string;
  rooms: { id: string; name: string }[];
  color: string;
  canAct: boolean;
  busy: boolean;
  onAct: (input: LiveInput) => void;
}) {
  const [open, setOpen] = useState<'delay' | 'move' | null>(null);
  const [minutes, setMinutes] = useState(15);
  const [push, setPush] = useState(true);
  const [moveRoom, setMoveRoom] = useState(item.room_id);
  const [moveTime, setMoveTime] = useState(() => timeIn(item.starts_at, tz));
  const start = new Date(item.starts_at).getTime();
  const end = new Date(item.ends_at).getTime();
  const running = now >= start && now < end && item.status === 'scheduled';
  const done = now >= end;
  const cancelled = item.status === 'cancelled';

  return (
    <div className={cn('px-4 py-3', running && 'bg-success/5', done && 'opacity-60')}>
      <div className="flex flex-wrap items-center gap-3">
        <div className="w-24 shrink-0 tabular-nums">
          <p className="text-sm font-semibold">{timeIn(item.starts_at, tz)}</p>
          <p className="text-xs text-muted-foreground">
            to {timeIn(item.ends_at, tz)} · {itemMinutes(item)}′
          </p>
        </div>
        <span className="h-10 w-1 shrink-0 rounded-full" style={{ backgroundColor: color }} />
        <div className="min-w-0 flex-1">
          <p className={cn('font-medium', cancelled && 'line-through')}>{item.title}</p>
          <p className="text-xs text-muted-foreground">
            {room}
            {item.people.length > 0 ? ` · ${item.people.map((p) => p.name).join(', ')}` : ''}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          {running ? <Pill tone="success">On now</Pill> : null}
          {cancelled ? <Pill tone="danger">Cancelled</Pill> : null}
          {item.staff_only ? <Pill>Staff only</Pill> : null}
          {canAct ? (
            cancelled ? (
              <Button size="sm" variant="secondary" disabled={busy} onClick={() => onAct({ action: 'restore' })}>
                <RotateCcw className="h-3.5 w-3.5" />
                Restore
              </Button>
            ) : (
              <>
                <Button size="sm" variant="secondary" disabled={busy} onClick={() => setOpen(open === 'delay' ? null : 'delay')}>
                  <Clock className="h-3.5 w-3.5" />
                  Delay
                </Button>
                <Button size="sm" variant="secondary" disabled={busy} onClick={() => setOpen(open === 'move' ? null : 'move')}>
                  <MoveRight className="h-3.5 w-3.5" />
                  Move
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-destructive"
                  disabled={busy}
                  onClick={() => {
                    if (confirm(`Cancel “${item.title}”? The public schedule shows it as cancelled at once.`)) onAct({ action: 'cancel' });
                  }}
                >
                  <Ban className="h-3.5 w-3.5" />
                  Cancel
                </Button>
              </>
            )
          ) : null}
        </div>
      </div>
      {open === 'delay' ? (
        <div className="mt-3 flex flex-wrap items-center gap-2 rounded-lg bg-secondary/60 px-3 py-2 text-sm">
          {[5, 10, 15, 30].map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setMinutes(m)}
              className={cn('rounded-pill px-2.5 py-1 text-xs font-semibold', minutes === m ? 'bg-primary text-primary-foreground' : 'bg-card')}
            >
              +{m}
            </button>
          ))}
          <Input
            type="number"
            aria-label="Minutes"
            className="h-8 w-20"
            value={minutes}
            onChange={(e) => setMinutes(Number(e.target.value) || 0)}
          />
          <span className="text-xs text-muted-foreground">min (negative = earlier)</span>
          <label className="flex items-center gap-1.5 text-xs">
            <input type="checkbox" checked={push} onChange={(e) => setPush(e.target.checked)} className="accent-primary" />
            Push the rest of {room}’s day too
          </label>
          <Button
            size="sm"
            className="ml-auto"
            disabled={busy || minutes === 0}
            onClick={() => {
              onAct({ action: 'delay', minutes, push_room: push });
              setOpen(null);
            }}
          >
            Apply delay
          </Button>
        </div>
      ) : null}
      {open === 'move' ? (
        <div className="mt-3 flex flex-wrap items-center gap-2 rounded-lg bg-secondary/60 px-3 py-2 text-sm">
          <select aria-label="New room" className={selectClasses} value={moveRoom} onChange={(e) => setMoveRoom(e.target.value)}>
            {rooms.map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </select>
          <Input type="time" aria-label="New start" className="h-9 w-28" step={300} value={moveTime} onChange={(e) => setMoveTime(e.target.value)} />
          <Button
            size="sm"
            className="ml-auto"
            disabled={busy}
            onClick={() => {
              const m = parseClock(moveTime);
              if (m === null) return;
              const startsAt = zonedToUtc(day, m, tz);
              onAct({
                action: 'move',
                ...(moveRoom !== item.room_id ? { room_id: moveRoom } : {}),
                ...(startsAt !== new Date(item.starts_at).toISOString() ? { starts_at: startsAt } : {}),
              });
              setOpen(null);
            }}
          >
            Move to {minutesLabel(parseClock(moveTime) ?? 0)}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function Announcements({ schedule: sc, canAct, onSetup }: { schedule: Schedule; canAct: boolean; onSetup: () => void }) {
  const qc = useQueryClient();
  const key = ['announcements', sc.edition.id];
  const list = useQuery({ queryKey: key, queryFn: () => listAnnouncements(sc.edition.id), refetchInterval: 30_000 });
  const refresh = () => void qc.invalidateQueries({ queryKey: key });
  const [text, setText] = useState('');
  const create = useMutation({
    mutationFn: () => createAnnouncement(sc.edition.id, text),
    onSuccess: () => {
      setText('');
      refresh();
    },
  });
  const items = list.data?.items ?? [];
  const drafts = items.filter((a) => a.status === 'draft');
  const history = items.filter((a) => a.status !== 'draft').slice(0, 15);

  return (
    <Card className="self-start">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Megaphone className="h-4 w-4" />
          Announcements
        </CardTitle>
        <p className="text-xs text-muted-foreground">
          {sc.telegram_configured
            ? 'Confirmed announcements are posted by the Telegram bot to the chats in Setup.'
            : 'No bot token is configured: sending records the message but posts nothing (dry run).'}{' '}
          <button type="button" className="text-primary hover:underline" onClick={onSetup}>
            Chats
          </button>
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        {drafts.length === 0 ? <p className="text-sm text-muted-foreground">Nothing waiting to be sent.</p> : null}
        {drafts.map((a) => (
          <DraftAnnouncement key={a.id} a={a} canAct={canAct} onDone={refresh} />
        ))}
        {canAct ? (
          <div className="space-y-2 border-t border-border pt-3">
            <textarea
              className={cn(textareaClasses, 'min-h-16 text-sm')}
              placeholder="Write a free announcement…"
              value={text}
              onChange={(e) => setText(e.target.value)}
            />
            <Button size="sm" variant="secondary" disabled={!text.trim() || create.isPending} onClick={() => create.mutate()}>
              Add as draft
            </Button>
            {create.error ? <p className="text-xs text-destructive">{errorText(create.error)}</p> : null}
          </div>
        ) : null}
        {history.length > 0 ? (
          <div className="space-y-2 border-t border-border pt-3">
            <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">History</h4>
            {history.map((a) => (
              <div key={a.id} className="rounded-md bg-secondary/50 px-3 py-2 text-xs">
                <p className="whitespace-pre-wrap">{a.body}</p>
                <p className="mt-1 text-muted-foreground">
                  {a.status === 'sent'
                    ? `Sent by ${a.sent_by || 'someone'} · ${a.result.map((r) => `${r.chat}: ${r.ok ? (r.dry_run ? 'dry run' : 'ok') : r.error}`).join(', ')}`
                    : 'Discarded'}
                </p>
              </div>
            ))}
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

function DraftAnnouncement({ a, canAct, onDone }: { a: Announcement; canAct: boolean; onDone: () => void }) {
  const [body, setBody] = useState(a.body);
  const save = useMutation({ mutationFn: () => updateAnnouncement(a.id, body), onSuccess: onDone });
  const send = useMutation({
    mutationFn: async () => {
      if (body !== a.body) await updateAnnouncement(a.id, body);
      return sendAnnouncement(a.id);
    },
    onSuccess: onDone,
  });
  const discard = useMutation({ mutationFn: () => discardAnnouncement(a.id), onSuccess: onDone });
  const failed = a.result.filter((r) => !r.ok);
  return (
    <div className="space-y-2 rounded-lg border border-warning/40 bg-warning/5 p-3">
      <textarea
        disabled={!canAct}
        className={cn(textareaClasses, 'min-h-20 text-sm')}
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onBlur={() => body !== a.body && save.mutate()}
      />
      {failed.length > 0 ? (
        <p className="text-xs text-destructive">Last try failed: {failed.map((r) => `${r.chat}: ${r.error}`).join(', ')}</p>
      ) : null}
      {canAct ? (
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            disabled={send.isPending || !body.trim()}
            onClick={() => {
              if (confirm('Post this announcement to the Telegram chats?')) send.mutate();
            }}
          >
            <Send className="h-3.5 w-3.5" />
            Send
          </Button>
          <Button size="sm" variant="ghost" disabled={discard.isPending} onClick={() => discard.mutate()}>
            <Trash2 className="h-3.5 w-3.5" />
            Discard
          </Button>
          <span className="ml-auto text-[11px] text-muted-foreground">by {a.created_by || 'someone'}</span>
        </div>
      ) : null}
      {send.error || save.error || discard.error ? (
        <p className="text-xs text-destructive">{errorText(send.error || save.error || discard.error)}</p>
      ) : null}
    </div>
  );
}
