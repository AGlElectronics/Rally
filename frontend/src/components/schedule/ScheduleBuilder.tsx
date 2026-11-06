import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, CalendarPlus, CheckCircle2, EyeOff, GripVertical, Plus, Send, Settings2, Undo2, Users, XCircle } from 'lucide-react';
import { errorText } from '@/api/client';
import {
  conflictsOf,
  createItem,
  deleteItem,
  publishSchedule,
  updateItem,
  type Conflict,
  type Schedule,
  type ScheduleItem,
} from '@/api/schedule';
import { ItemPanel, type PanelTarget } from '@/components/schedule/ItemPanel';
import {
  byId,
  conflictsByItem,
  itemMinutes,
  scheduleDays,
  typeColor,
  usePutSchedule,
  worstSeverity,
} from '@/components/schedule/shared';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { PERMS, useMe } from '@/hooks/useMe';
import { cn } from '@/lib/utils';
import { dayLabel, minutesLabel, zonedParts, zonedToUtc } from '@/lib/zonedTime';

/** Pixels per minute on the grid. */
const PX = 1.6;
const DAY_KEY = 'rally.scheduleDay';

type Drag =
  | { kind: 'new'; submissionId: string; minutes: number; grab: number }
  | { kind: 'move'; itemId: string; minutes: number; grab: number };

type Pos = { roomId: string; start: number; end: number };

type Notice = { text: string; conflicts: Conflict[] } | null;

export function ScheduleBuilder({ schedule: sc, onSetup }: { schedule: Schedule; onSetup: () => void }) {
  const { can } = useMe();
  const canEdit = can(PERMS.scheduleEdit);
  const canPublish = can(PERMS.schedulePublish);
  const tz = sc.edition.timezone;
  const put = usePutSchedule(sc.edition.id);
  const days = useMemo(() => scheduleDays(sc), [sc]);
  const [day, setDayState] = useState(() => localStorage.getItem(DAY_KEY) ?? '');
  const current = days.includes(day) ? day : days[0];
  const setDay = (d: string) => {
    localStorage.setItem(DAY_KEY, d);
    setDayState(d);
  };

  const [notice, setNotice] = useState<Notice>(null);
  const [pending, setPending] = useState<Record<string, Pos>>({});
  const [ghost, setGhost] = useState<(Pos & { valid: boolean }) | null>(null);
  const [panel, setPanel] = useState<PanelTarget | null>(null);
  const [resize, setResize] = useState<{ id: string; pos: Pos } | null>(null);
  const [overSidebar, setOverSidebar] = useState(false);
  const [note, setNote] = useState('');
  const [publishing, setPublishing] = useState(false);
  const drag = useRef<Drag | null>(null);

  const types = useMemo(() => byId(sc.event_types), [sc.event_types]);
  const conflicts = useMemo(() => conflictsByItem(sc.conflicts), [sc.conflicts]);
  const blocking = sc.conflicts.filter((c) => c.severity === 'block').length;
  const warnings = sc.conflicts.length - blocking;
  const snap = sc.settings.snap_minutes;

  // Items of the chosen day with their local minutes; pending moves win.
  const placed = useMemo(() => {
    return sc.items
      .map((it) => {
        const p = zonedParts(it.starts_at, tz);
        const pos: Pos = pending[it.id] ?? { roomId: it.room_id, start: p.minutes, end: p.minutes + itemMinutes(it) };
        return { it, date: p.date, pos };
      })
      .filter((x) => x.date === current);
  }, [sc.items, tz, pending, current]);

  const gridStart = Math.min(sc.settings.day_start_minute, ...placed.map((p) => Math.floor(p.pos.start / 60) * 60));
  const gridEnd = Math.max(sc.settings.day_end_minute, ...placed.map((p) => Math.ceil(p.pos.end / 60) * 60));
  const height = (gridEnd - gridStart) * PX;

  async function run(fn: () => Promise<Schedule>, optimistic?: { id: string; pos: Pos }) {
    if (optimistic) setPending((p) => ({ ...p, [optimistic.id]: optimistic.pos }));
    try {
      put(await fn());
      setNotice(null);
      return true;
    } catch (e) {
      setNotice({ text: errorText(e), conflicts: conflictsOf(e) });
      return false;
    } finally {
      if (optimistic)
        setPending((p) => {
          const next = { ...p };
          delete next[optimistic.id];
          return next;
        });
    }
  }

  const snapTo = (m: number) => Math.round(m / snap) * snap;
  const minuteAt = (clientY: number, el: HTMLElement, grab: number) => {
    const rect = el.getBoundingClientRect();
    const raw = gridStart + (clientY - rect.top) / PX - grab;
    return Math.max(gridStart, Math.min(gridEnd - snap, snapTo(raw)));
  };

  const onColumnDragOver = (e: React.DragEvent<HTMLDivElement>, roomId: string) => {
    const d = drag.current;
    if (!d || !canEdit) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    const start = minuteAt(e.clientY, e.currentTarget, d.grab);
    if (!ghost || ghost.roomId !== roomId || ghost.start !== start) {
      setGhost({ roomId, start, end: start + d.minutes, valid: true });
    }
  };

  const onColumnDrop = (e: React.DragEvent<HTMLDivElement>, roomId: string) => {
    const d = drag.current;
    e.preventDefault();
    setGhost(null);
    drag.current = null;
    if (!d || !canEdit) return;
    const start = minuteAt(e.clientY, e.currentTarget, d.grab);
    const startsAt = zonedToUtc(current, start, tz);
    if (d.kind === 'new') {
      void run(() => createItem(sc.edition.id, { submission_id: d.submissionId, room_id: roomId, starts_at: startsAt }));
    } else {
      void run(() => updateItem(d.itemId, { room_id: roomId, starts_at: startsAt }), {
        id: d.itemId,
        pos: { roomId, start, end: start + d.minutes },
      });
    }
  };

  // Resize by dragging an item's bottom edge.
  const resizeRef = useRef(resize);
  resizeRef.current = resize;
  useEffect(() => {
    if (!resize) return;
    const move = (e: PointerEvent) => {
      const r = resizeRef.current;
      const el = r && document.querySelector<HTMLElement>(`[data-room-col="${r.pos.roomId}"]`);
      if (!r || !el) return;
      const rect = el.getBoundingClientRect();
      const end = Math.max(r.pos.start + snap, Math.min(gridEnd, snapTo(gridStart + (e.clientY - rect.top) / PX)));
      if (end !== r.pos.end) setResize({ ...r, pos: { ...r.pos, end } });
    };
    const up = () => {
      const r = resizeRef.current;
      setResize(null);
      if (!r) return;
      const it = sc.items.find((x) => x.id === r.id);
      const original = it ? zonedParts(it.starts_at, tz).minutes + itemMinutes(it) : null;
      if (r.pos.end !== original) {
        void run(() => updateItem(r.id, { ends_at: zonedToUtc(current, r.pos.end, tz) }), { id: r.id, pos: r.pos });
      }
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up, { once: true });
    return () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resize?.id]);

  const publish = async () => {
    setPublishing(true);
    const ok = await run(() => publishSchedule(sc.edition.id, note));
    if (ok) setNote('');
    setPublishing(false);
  };

  if (sc.rooms.length === 0 || days.length === 0) {
    return (
      <Card className="flex flex-col items-start gap-3 p-6">
        <p className="font-semibold">Set up the schedule first</p>
        <p className="text-sm text-muted-foreground">
          {sc.rooms.length === 0 ? 'Add the rooms or stages. ' : ''}
          {days.length === 0 ? 'Set the edition’s dates and timezone so the builder knows which days to show.' : ''}
        </p>
        <Button onClick={onSetup}>
          <Settings2 className="h-4 w-4" />
          Open setup
        </Button>
      </Card>
    );
  }

  const hours: number[] = [];
  for (let m = Math.ceil(gridStart / 60) * 60; m <= gridEnd; m += 60) hours.push(m);

  return (
    <div className="space-y-3">
      <Card className="flex flex-wrap items-center gap-3 px-4 py-3">
        <div className="flex items-center gap-2 text-sm">
          {blocking > 0 ? (
            <span className="inline-flex items-center gap-1 font-semibold text-destructive">
              <XCircle className="h-4 w-4" /> {blocking} blocking
            </span>
          ) : (
            <span className="inline-flex items-center gap-1 text-success">
              <CheckCircle2 className="h-4 w-4" /> No blocking conflicts
            </span>
          )}
          {warnings > 0 ? (
            <span className="inline-flex items-center gap-1 text-warning">
              <AlertTriangle className="h-4 w-4" /> {warnings} warning{warnings === 1 ? '' : 's'}
            </span>
          ) : null}
        </div>
        <span className="text-xs text-muted-foreground">
          {sc.published
            ? sc.draft_changed
              ? `The public sees version ${sc.published.version}; your draft has newer changes.`
              : `The public sees version ${sc.published.version}, same as this draft.`
            : 'Nothing is public yet.'}
          {sc.public_enabled ? '' : ' The public schedule API is switched off.'}
        </span>
        {canPublish ? (
          <div className="ml-auto flex items-center gap-2">
            <Input
              placeholder="What changed? (optional)"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              className="h-8 w-56 text-xs"
            />
            <Button
              size="sm"
              onClick={() => void publish()}
              disabled={publishing || blocking > 0 || (!!sc.published && !sc.draft_changed)}
              title={blocking > 0 ? 'Fix the blocking conflicts first' : undefined}
            >
              <Send className="h-3.5 w-3.5" />
              Publish
            </Button>
          </div>
        ) : null}
      </Card>

      {notice ? (
        <div className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-2.5 text-sm">
          <XCircle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
          <div className="flex-1">
            <p className="font-semibold">{notice.text}</p>
            {notice.conflicts.length > 1 ? (
              <ul className="mt-1 list-disc pl-4 text-xs text-muted-foreground">
                {notice.conflicts.map((c, i) => (
                  <li key={i}>{c.message}</li>
                ))}
              </ul>
            ) : null}
          </div>
          <button type="button" className="text-xs text-muted-foreground hover:text-foreground" onClick={() => setNotice(null)}>
            Dismiss
          </button>
        </div>
      ) : null}

      <div className="flex flex-wrap gap-1">
        {days.map((d) => (
          <button
            key={d}
            type="button"
            onClick={() => setDay(d)}
            className={cn(
              'rounded-pill px-3 py-1.5 text-sm font-medium transition-colors',
              d === current ? 'bg-primary text-primary-foreground' : 'bg-secondary text-muted-foreground hover:text-foreground',
            )}
          >
            {dayLabel(d)}
          </button>
        ))}
      </div>

      <div className="flex flex-col gap-3 lg:flex-row lg:items-start">
        <aside
          className={cn(
            'w-full shrink-0 space-y-3 rounded-xl border border-border bg-card p-3 lg:sticky lg:top-3 lg:w-72',
            overSidebar && 'border-primary ring-2 ring-primary-soft',
          )}
          onDragOver={(e) => {
            if (drag.current?.kind === 'move') {
              e.preventDefault();
              setOverSidebar(true);
              setGhost(null);
            }
          }}
          onDragLeave={() => setOverSidebar(false)}
          onDrop={(e) => {
            e.preventDefault();
            setOverSidebar(false);
            const d = drag.current;
            drag.current = null;
            if (d?.kind === 'move' && canEdit) void run(() => deleteItem(d.itemId));
          }}
        >
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold">To schedule ({sc.unscheduled.length})</h3>
            {canEdit ? (
              <Button size="sm" variant="secondary" onClick={() => setPanel({ mode: 'new', day: current })}>
                <Plus className="h-3.5 w-3.5" />
                Add item
              </Button>
            ) : null}
          </div>
          {sc.unscheduled.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              Every accepted submission is on the grid. Use “Add item” for breaks, ceremonies or staff blocks.
            </p>
          ) : (
            <ul className="max-h-[45vh] space-y-1.5 overflow-y-auto pr-1">
              {sc.unscheduled.map((u) => (
                <li
                  key={u.id}
                  draggable={canEdit}
                  onDragStart={(e) => {
                    drag.current = { kind: 'new', submissionId: u.id, minutes: u.duration_minutes, grab: 0 };
                    e.dataTransfer.setData('text/plain', u.id);
                    e.dataTransfer.effectAllowed = 'move';
                  }}
                  onDragEnd={() => {
                    drag.current = null;
                    setGhost(null);
                  }}
                  className={cn(
                    'rounded-lg border border-border bg-background p-2 text-sm',
                    canEdit && 'cursor-grab active:cursor-grabbing hover:border-primary',
                  )}
                  style={{ borderLeft: `4px solid ${typeColor(types, u.event_type_id)}` }}
                >
                  <div className="flex items-start gap-1">
                    {canEdit ? <GripVertical className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" /> : null}
                    <div className="min-w-0">
                      <p className="font-medium leading-snug">{u.title}</p>
                      <p className="text-xs text-muted-foreground">
                        {(u.event_type_id && types.get(u.event_type_id)?.name) || 'No type'} · {u.duration_minutes} min
                      </p>
                      {u.people.length > 0 ? (
                        <p className="truncate text-xs text-muted-foreground">{u.people.map((p) => p.name).join(', ')}</p>
                      ) : null}
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          )}
          {canEdit ? (
            <p className="rounded-md border border-dashed border-border px-2 py-1.5 text-center text-[11px] text-muted-foreground">
              Drag an item here to take it off the schedule
            </p>
          ) : null}

          {sc.conflicts.length > 0 ? (
            <div className="space-y-1.5 border-t border-border pt-3">
              <h3 className="text-sm font-semibold">Conflicts</h3>
              <ul className="max-h-[30vh] space-y-1 overflow-y-auto pr-1">
                {sc.conflicts.map((c, i) => {
                  const it = sc.items.find((x) => x.id === c.item_id);
                  return (
                    <li key={i}>
                      <button
                        type="button"
                        className="w-full rounded-md px-2 py-1 text-left text-xs hover:bg-secondary"
                        onClick={() => {
                          if (it) {
                            setDay(zonedParts(it.starts_at, tz).date);
                            setPanel({ mode: 'edit', item: it });
                          }
                        }}
                      >
                        <span className={c.severity === 'block' ? 'font-semibold text-destructive' : 'font-semibold text-warning'}>
                          {c.severity === 'block' ? 'Blocks' : 'Warning'}:
                        </span>{' '}
                        {c.message}
                      </button>
                    </li>
                  );
                })}
              </ul>
            </div>
          ) : null}
        </aside>

        <Card className="min-w-0 flex-1 overflow-x-auto">
          <div className="flex min-w-max">
            <div className="sticky left-0 z-20 w-14 shrink-0 border-r border-border bg-card">
              <div className="h-10 border-b border-border" />
              <div className="relative" style={{ height }}>
                {hours.map((m) => (
                  <span
                    key={m}
                    className="absolute right-2 -translate-y-1/2 text-[11px] tabular-nums text-muted-foreground"
                    style={{ top: (m - gridStart) * PX }}
                  >
                    {minutesLabel(m)}
                  </span>
                ))}
              </div>
            </div>
            {sc.rooms.map((room) => {
              const inRoom = placed.filter((p) => (resize?.id === p.it.id ? resize.pos : p.pos).roomId === room.id);
              const lanes = assignLanes(inRoom.map((p) => ({ id: p.it.id, ...(resize?.id === p.it.id ? resize.pos : p.pos) })));
              return (
                <div key={room.id} className="w-48 shrink-0 border-r border-border last:border-r-0">
                  <div className="sticky top-0 z-10 flex h-10 items-center justify-between gap-1 border-b border-border bg-card px-2">
                    <span className="truncate text-sm font-semibold">{room.name}</span>
                    {room.capacity ? <span className="text-[11px] text-muted-foreground">{room.capacity}</span> : null}
                  </div>
                  <div
                    data-room-col={room.id}
                    className="relative"
                    style={{ height }}
                    onDragOver={(e) => onColumnDragOver(e, room.id)}
                    onDragLeave={(e) => {
                      if (!e.currentTarget.contains(e.relatedTarget as Node)) setGhost(null);
                    }}
                    onDrop={(e) => onColumnDrop(e, room.id)}
                    onDoubleClick={(e) => {
                      if (!canEdit || e.target !== e.currentTarget) return;
                      const start = minuteAt(e.clientY, e.currentTarget, 0);
                      setPanel({ mode: 'new', day: current, roomId: room.id, start });
                    }}
                  >
                    {hours.map((m) => (
                      <div key={m} className="absolute inset-x-0 border-t border-border/60" style={{ top: (m - gridStart) * PX }} />
                    ))}
                    {hours.map((m) => (
                      <div
                        key={`h${m}`}
                        className="absolute inset-x-0 border-t border-dashed border-border/30"
                        style={{ top: (m + 30 - gridStart) * PX }}
                      />
                    ))}
                    {ghost && ghost.roomId === room.id ? (
                      <div
                        className="pointer-events-none absolute inset-x-1 z-10 rounded-md border-2 border-dashed border-primary bg-primary-soft/60 px-1.5 text-[11px] font-semibold text-primary"
                        style={{ top: (ghost.start - gridStart) * PX, height: (ghost.end - ghost.start) * PX }}
                      >
                        {minutesLabel(ghost.start)}–{minutesLabel(ghost.end)}
                      </div>
                    ) : null}
                    {inRoom.map(({ it, pos: p }) => {
                      const pos = resize?.id === it.id ? resize.pos : p;
                      const lane = lanes.get(it.id) ?? { lane: 0, of: 1 };
                      return (
                        <GridItem
                          key={it.id}
                          item={it}
                          pos={pos}
                          top={(pos.start - gridStart) * PX}
                          lane={lane}
                          color={typeColor(types, it.event_type_id)}
                          severity={worstSeverity(conflicts.get(it.id))}
                          pending={!!pending[it.id]}
                          canEdit={canEdit}
                          onOpen={() => setPanel({ mode: 'edit', item: it })}
                          onDragStart={(e) => {
                            const rect = e.currentTarget.getBoundingClientRect();
                            drag.current = {
                              kind: 'move',
                              itemId: it.id,
                              minutes: pos.end - pos.start,
                              grab: (e.clientY - rect.top) / PX,
                            };
                            e.dataTransfer.setData('text/plain', it.id);
                            e.dataTransfer.effectAllowed = 'move';
                          }}
                          onDragEnd={() => {
                            drag.current = null;
                            setGhost(null);
                            setOverSidebar(false);
                          }}
                          onResizeStart={() => setResize({ id: it.id, pos })}
                        />
                      );
                    })}
                  </div>
                </div>
              );
            })}
          </div>
        </Card>
      </div>
      {canEdit ? (
        <p className="text-xs text-muted-foreground">
          Drag cards onto a room, drag blocks to move them, pull the bottom edge to change the length, double-click empty space
          to add an item. Times are in {tz}.
        </p>
      ) : null}

      {panel ? (
        <ItemPanel
          schedule={sc}
          target={panel}
          days={days}
          canEdit={canEdit}
          conflicts={panel.mode === 'edit' ? conflicts.get(panel.item.id) ?? [] : []}
          onClose={() => setPanel(null)}
          onSaved={(next) => {
            put(next);
            setPanel(null);
          }}
        />
      ) : null}
    </div>
  );
}

function GridItem({
  item,
  pos,
  top,
  lane,
  color,
  severity,
  pending,
  canEdit,
  onOpen,
  onDragStart,
  onDragEnd,
  onResizeStart,
}: {
  item: ScheduleItem;
  pos: Pos;
  top: number;
  lane: { lane: number; of: number };
  color: string;
  severity: 'block' | 'warn' | null;
  pending: boolean;
  canEdit: boolean;
  onOpen: () => void;
  onDragStart: (e: React.DragEvent<HTMLDivElement>) => void;
  onDragEnd: () => void;
  onResizeStart: () => void;
}) {
  const h = Math.max(18, (pos.end - pos.start) * PX - 2);
  const width = 100 / lane.of;
  return (
    <div
      draggable={canEdit}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onClick={onOpen}
      title={`${item.title} · ${minutesLabel(pos.start)}–${minutesLabel(pos.end)}`}
      className={cn(
        'group absolute z-[5] overflow-hidden rounded-md border px-1.5 py-1 text-left text-xs shadow-sm transition-shadow hover:z-[6] hover:shadow-md',
        canEdit ? 'cursor-grab active:cursor-grabbing' : 'cursor-pointer',
        severity === 'block' ? 'ring-2 ring-destructive' : severity === 'warn' ? 'ring-2 ring-warning' : '',
        item.status === 'cancelled' && 'opacity-50',
        pending && 'opacity-70',
      )}
      style={{
        top,
        height: h,
        left: `calc(${lane.lane * width}% + 2px)`,
        width: `calc(${width}% - 4px)`,
        backgroundColor: `color-mix(in srgb, ${color} 18%, hsl(var(--card)))`,
        borderColor: `color-mix(in srgb, ${color} 55%, transparent)`,
        borderLeft: `4px solid ${color}`,
      }}
    >
      <p className={cn('truncate font-semibold leading-tight', item.status === 'cancelled' && 'line-through')}>{item.title}</p>
      {h > 30 ? (
        <p className="truncate text-[11px] text-muted-foreground">
          {minutesLabel(pos.start)}–{minutesLabel(pos.end)}
          {item.staff_only ? ' · staff only' : ''}
        </p>
      ) : null}
      {h > 48 && item.people.length > 0 ? (
        <p className="flex items-center gap-1 truncate text-[11px] text-muted-foreground">
          <Users className="h-3 w-3 shrink-0" />
          {item.people.map((p) => p.name).join(', ')}
        </p>
      ) : null}
      <span className="absolute right-1 top-1 flex gap-0.5">
        {item.staff_only ? <EyeOff className="h-3 w-3 text-muted-foreground" /> : null}
        {item.submission_id ? null : <CalendarPlus className="h-3 w-3 text-muted-foreground" />}
        {severity ? <AlertTriangle className={cn('h-3 w-3', severity === 'block' ? 'text-destructive' : 'text-warning')} /> : null}
        {item.status === 'cancelled' ? <Undo2 className="h-3 w-3 text-muted-foreground" /> : null}
      </span>
      {canEdit ? (
        <div
          className="absolute inset-x-0 bottom-0 h-2 cursor-ns-resize opacity-0 group-hover:opacity-100"
          style={{ background: `linear-gradient(transparent, ${color})` }}
          onMouseDown={(e) => e.preventDefault()}
          onClick={(e) => e.stopPropagation()}
          onDragStart={(e) => {
            e.preventDefault();
            e.stopPropagation();
          }}
          onPointerDown={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onResizeStart();
          }}
        />
      ) : null}
    </div>
  );
}

/** Puts overlapping items side by side: lane index and lane count per item. */
function assignLanes(items: { id: string; start: number; end: number }[]): Map<string, { lane: number; of: number }> {
  const sorted = [...items].sort((a, b) => a.start - b.start || a.end - b.end);
  const out = new Map<string, { lane: number; of: number }>();
  let cluster: { id: string; lane: number }[] = [];
  let clusterEnd = -Infinity;
  let laneEnds: number[] = [];
  const flush = () => {
    const of = Math.max(1, laneEnds.length);
    for (const c of cluster) out.set(c.id, { lane: c.lane, of });
    cluster = [];
    laneEnds = [];
  };
  for (const it of sorted) {
    if (it.start >= clusterEnd) flush();
    let lane = laneEnds.findIndex((end) => end <= it.start);
    if (lane < 0) {
      lane = laneEnds.length;
      laneEnds.push(it.end);
    } else {
      laneEnds[lane] = it.end;
    }
    cluster.push({ id: it.id, lane });
    clusterEnd = Math.max(clusterEnd === -Infinity ? it.end : clusterEnd, it.end);
  }
  flush();
  return out;
}
