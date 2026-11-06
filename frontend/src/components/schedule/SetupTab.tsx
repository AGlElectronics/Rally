import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Bot, Copy, Globe, KeyRound, Plus, Trash2 } from 'lucide-react';
import { errorText } from '@/api/client';
import {
  RULES,
  createApiKey,
  createChat,
  createRoom,
  deleteChat,
  deleteRoom,
  listApiKeys,
  listChats,
  revokeApiKey,
  updateChat,
  updateEditionDates,
  updateRoom,
  updateScheduleSettings,
  type Room,
  type RuleMode,
  type Schedule,
  type ScheduleSettings,
  type TelegramChat,
} from '@/api/schedule';
import { usePutSchedule } from '@/components/schedule/shared';
import { Button } from '@/components/ui/Button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/Card';
import { Input, Label } from '@/components/ui/Input';
import { Pill, selectClasses } from '@/components/ui/Pill';
import { PERMS, useMe } from '@/hooks/useMe';
import { minutesLabel, parseClock, timezones, zonedParts, zonedToUtc } from '@/lib/zonedTime';

export function SetupTab({ schedule: sc }: { schedule: Schedule }) {
  const me = useMe();
  const canEdit = me.can(PERMS.scheduleEdit);
  const canPublish = me.can(PERMS.schedulePublish);
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <DatesCard schedule={sc} canEdit={canEdit} />
      <GridCard schedule={sc} canEdit={canEdit} />
      <RoomsCard schedule={sc} canEdit={canEdit} />
      <div className="space-y-4">
        <PublicCard schedule={sc} canPublish={canPublish} />
        {canPublish ? <ChatsCard schedule={sc} /> : null}
      </div>
    </div>
  );
}

function Status({ error, saved }: { error: unknown; saved: boolean }) {
  if (error) return <span className="text-xs text-destructive">{errorText(error)}</span>;
  return saved ? <span className="text-xs text-success">Saved</span> : null;
}

function DatesCard({ schedule: sc, canEdit }: { schedule: Schedule; canEdit: boolean }) {
  const qc = useQueryClient();
  const ed = sc.edition;
  const [tz, setTz] = useState(ed.timezone || 'UTC');
  const [start, setStart] = useState(ed.start_at ? zonedParts(ed.start_at, ed.timezone).date : '');
  const [end, setEnd] = useState(ed.end_at ? zonedParts(ed.end_at, ed.timezone).date : '');
  const save = useMutation({
    mutationFn: () =>
      updateEditionDates(ed.id, {
        timezone: tz,
        start_at: start ? zonedToUtc(start, 0, tz) : null,
        end_at: end ? zonedToUtc(end, 1439, tz) : null,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['schedule', ed.id] });
      void qc.invalidateQueries({ queryKey: ['editions'] });
    },
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle>Dates</CardTitle>
        <CardDescription>The days of {ed.title}. Each day becomes a column set on the grid.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1">
            <Label htmlFor="ed-start">First day</Label>
            <Input id="ed-start" type="date" disabled={!canEdit} value={start} onChange={(e) => setStart(e.target.value)} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="ed-end">Last day</Label>
            <Input id="ed-end" type="date" disabled={!canEdit} value={end} min={start} onChange={(e) => setEnd(e.target.value)} />
          </div>
        </div>
        <div className="space-y-1">
          <Label htmlFor="ed-tz">Timezone</Label>
          <select id="ed-tz" className={selectClasses + ' w-full'} disabled={!canEdit} value={tz} onChange={(e) => setTz(e.target.value)}>
            {[...new Set([tz, ...timezones()])].map((z) => (
              <option key={z} value={z}>
                {z}
              </option>
            ))}
          </select>
        </div>
        {canEdit ? (
          <div className="flex items-center gap-3">
            <Button size="sm" disabled={save.isPending || !!(start && end && end < start)} onClick={() => save.mutate()}>
              Save dates
            </Button>
            <Status error={save.error} saved={save.isSuccess} />
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

function GridCard({ schedule: sc, canEdit }: { schedule: Schedule; canEdit: boolean }) {
  const put = usePutSchedule(sc.edition.id);
  const [s, setS] = useState<ScheduleSettings>(sc.settings);
  const [from, setFrom] = useState(minutesLabel(sc.settings.day_start_minute));
  const [to, setTo] = useState(sc.settings.day_end_minute >= 1440 ? '24:00' : minutesLabel(sc.settings.day_end_minute));
  const save = useMutation({
    mutationFn: () => {
      const a = parseClock(from);
      const b = parseClock(to);
      return updateScheduleSettings(sc.edition.id, { ...s, day_start_minute: a ?? s.day_start_minute, day_end_minute: b ?? s.day_end_minute });
    },
    onSuccess: put,
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle>Grid and rules</CardTitle>
        <CardDescription>Hours shown on the grid, the snap step, and how strictly each rule is enforced.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-3">
          <div className="space-y-1">
            <Label htmlFor="g-from">Day starts</Label>
            <Input id="g-from" type="time" step={900} disabled={!canEdit} value={from} onChange={(e) => setFrom(e.target.value)} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="g-to">Day ends</Label>
            <Input id="g-to" disabled={!canEdit} value={to} placeholder="23:00" onChange={(e) => setTo(e.target.value)} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="g-snap">Snap to</Label>
            <select
              id="g-snap"
              className={selectClasses + ' w-full'}
              disabled={!canEdit}
              value={s.snap_minutes}
              onChange={(e) => setS({ ...s, snap_minutes: Number(e.target.value) })}
            >
              {[5, 10, 15, 30].map((m) => (
                <option key={m} value={m}>
                  {m} min
                </option>
              ))}
            </select>
          </div>
        </div>
        <div className="divide-y divide-border rounded-lg border border-border">
          {RULES.map((r) => (
            <div key={r.id} className="flex items-center gap-3 px-3 py-2">
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">{r.label}</p>
                <p className="text-xs text-muted-foreground">{r.hint}</p>
              </div>
              <select
                aria-label={r.label}
                className={selectClasses}
                disabled={!canEdit}
                value={s.rules[r.id]}
                onChange={(e) => setS({ ...s, rules: { ...s.rules, [r.id]: e.target.value as RuleMode } })}
              >
                <option value="block">Block</option>
                <option value="warn">Warn</option>
                <option value="off">Off</option>
              </select>
            </div>
          ))}
        </div>
        <p className="text-xs text-muted-foreground">
          Changeover gaps are set per event type (Event types page), as are the rooms a type may use.
        </p>
        {canEdit ? (
          <div className="flex items-center gap-3">
            <Button size="sm" disabled={save.isPending || parseClock(from) === null || parseClock(to) === null} onClick={() => save.mutate()}>
              Save grid settings
            </Button>
            <Status error={save.error} saved={save.isSuccess} />
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

function RoomsCard({ schedule: sc, canEdit }: { schedule: Schedule; canEdit: boolean }) {
  const qc = useQueryClient();
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['schedule', sc.edition.id] });
    void qc.invalidateQueries({ queryKey: ['rooms', sc.edition.id] });
  };
  const [name, setName] = useState('');
  const [cap, setCap] = useState('');
  const add = useMutation({
    mutationFn: () =>
      createRoom(sc.edition.id, {
        name: name.trim(),
        capacity: cap ? Number(cap) : null,
        sort_order: (sc.rooms.at(-1)?.sort_order ?? 0) + 10,
      }),
    onSuccess: () => {
      setName('');
      setCap('');
      refresh();
    },
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle>Rooms</CardTitle>
        <CardDescription>Each room is a column on the grid, in this order.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {sc.rooms.length === 0 ? <p className="text-sm text-muted-foreground">No rooms yet.</p> : null}
        {sc.rooms.map((r) => (
          <RoomRow key={`${r.id}-${r.name}-${r.capacity}-${r.sort_order}`} room={r} canEdit={canEdit} onDone={refresh} />
        ))}
        {canEdit ? (
          <form
            className="flex flex-wrap items-center gap-2 border-t border-border pt-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (name.trim()) add.mutate();
            }}
          >
            <Input aria-label="New room name" className="min-w-40 flex-1" placeholder="Room name" value={name} onChange={(e) => setName(e.target.value)} />
            <Input
              aria-label="New room capacity"
              className="w-28"
              type="number"
              min={0}
              placeholder="Capacity"
              value={cap}
              onChange={(e) => setCap(e.target.value)}
            />
            <Button size="sm" type="submit" disabled={!name.trim() || add.isPending}>
              <Plus className="h-3.5 w-3.5" />
              Add room
            </Button>
            {add.error ? <p className="w-full text-xs text-destructive">{errorText(add.error)}</p> : null}
          </form>
        ) : null}
      </CardContent>
    </Card>
  );
}

function RoomRow({ room, canEdit, onDone }: { room: Room; canEdit: boolean; onDone: () => void }) {
  const [name, setName] = useState(room.name);
  const [cap, setCap] = useState(room.capacity === null ? '' : String(room.capacity));
  const [order, setOrder] = useState(String(room.sort_order));
  const dirty = name !== room.name || cap !== (room.capacity === null ? '' : String(room.capacity)) || order !== String(room.sort_order);
  const save = useMutation({
    mutationFn: () => updateRoom(room.id, { name: name.trim(), capacity: cap ? Number(cap) : null, sort_order: Number(order) || 0 }),
    onSuccess: onDone,
  });
  const del = useMutation({ mutationFn: () => deleteRoom(room.id), onSuccess: onDone });
  return (
    <div className="space-y-1">
      <div className="flex flex-wrap items-center gap-2">
        <Input aria-label="Room name" className="min-w-40 flex-1" disabled={!canEdit} value={name} onChange={(e) => setName(e.target.value)} />
        <Input
          aria-label="Capacity"
          className="w-24"
          type="number"
          min={0}
          placeholder="Capacity"
          disabled={!canEdit}
          value={cap}
          onChange={(e) => setCap(e.target.value)}
        />
        <Input aria-label="Order" title="Order" className="w-20" type="number" disabled={!canEdit} value={order} onChange={(e) => setOrder(e.target.value)} />
        {canEdit ? (
          <>
            <Button size="sm" variant="secondary" disabled={!dirty || !name.trim() || save.isPending} onClick={() => save.mutate()}>
              Save
            </Button>
            <Button
              size="icon"
              variant="ghost"
              aria-label={`Delete ${room.name}`}
              disabled={del.isPending}
              onClick={() => {
                if (confirm(`Delete the room “${room.name}”?`)) del.mutate();
              }}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </>
        ) : null}
      </div>
      {save.error || del.error ? <p className="text-xs text-destructive">{errorText(save.error || del.error)}</p> : null}
    </div>
  );
}

function PublicCard({ schedule: sc, canPublish }: { schedule: Schedule; canPublish: boolean }) {
  const base = `${window.location.origin}/api/public/v1/schedules/${sc.edition.slug}`;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Globe className="h-4 w-4" />
          Public schedule API
          {sc.public_enabled ? <Pill tone="success">On</Pill> : <Pill>Off</Pill>}
        </CardTitle>
        <CardDescription>
          Serves the latest published version only (staff-only items left out), read-only, cached for a minute.
          Every consumer authenticates with an API key.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        {sc.public_enabled ? (
          <>
            <p>
              JSON:{' '}
              <a
                className="break-all text-primary hover:underline"
                href={`${base}?key=<your key>`}
                onClick={(e) => e.preventDefault()}
              >
                {base}
              </a>
            </p>
            <p>
              Calendar: <span className="break-all">{base}/calendar.ics?key=&lt;your key&gt;</span>
            </p>
            <p className="text-xs text-muted-foreground">
              Send the key as <code>Authorization: Bearer &lt;key&gt;</code>, or as <code>?key=</code> for calendar
              subscriptions (they cannot send headers).
            </p>
            {!sc.published ? <p className="text-xs text-warning">Nothing is published yet, so these return 404 for now.</p> : null}
          </>
        ) : null}
        <p className="text-xs text-muted-foreground">
          Switched on or off in Den’s organisation settings (<code>events_public_schedule</code>); without that setting, the{' '}
          <code>PUBLIC_SCHEDULE_ENABLED</code> config decides.
        </p>
        {canPublish ? <ApiKeys /> : null}
      </CardContent>
    </Card>
  );
}

/** Keys for the public schedule API; each consumer (website, info screen…)
 * gets its own, so one can be revoked without breaking the rest. */
function ApiKeys() {
  const qc = useQueryClient();
  const [name, setName] = useState('');
  const [fresh, setFresh] = useState<{ name: string; token: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const keys = useQuery({ queryKey: ['public-api-keys'], queryFn: listApiKeys });
  const refresh = () => void qc.invalidateQueries({ queryKey: ['public-api-keys'] });
  const create = useMutation({
    mutationFn: () => createApiKey(name.trim()),
    onSuccess: ({ key, token }) => {
      setFresh({ name: key.name, token });
      setCopied(false);
      setName('');
      refresh();
    },
  });
  const revoke = useMutation({ mutationFn: revokeApiKey, onSuccess: refresh });
  const active = (keys.data?.items ?? []).filter((k) => !k.revoked_at);

  return (
    <div className="space-y-2 border-t border-border pt-3">
      <p className="flex items-center gap-1.5 font-medium">
        <KeyRound className="h-3.5 w-3.5" /> API keys
      </p>
      {fresh ? (
        <div className="space-y-1 rounded-md border border-warning/50 bg-warning/10 p-2">
          <p className="text-xs">
            Key for <strong>{fresh.name}</strong> — copy it now, it is shown only once:
          </p>
          <div className="flex items-center gap-2">
            <Input readOnly value={fresh.token} className="flex-1 text-xs" aria-label="New API key" />
            <Button
              size="sm"
              variant="secondary"
              onClick={() => {
                void navigator.clipboard.writeText(fresh.token).then(() => setCopied(true));
              }}
            >
              <Copy className="h-3.5 w-3.5" /> {copied ? 'Copied' : 'Copy'}
            </Button>
          </div>
        </div>
      ) : null}
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim()) create.mutate();
        }}
      >
        <Input
          value={name}
          placeholder="Who gets this key? (e.g. Main website)"
          className="flex-1 text-xs"
          aria-label="Key name"
          onChange={(e) => setName(e.target.value)}
        />
        <Button type="submit" size="sm" disabled={!name.trim() || create.isPending}>
          <Plus className="h-3.5 w-3.5" /> Key
        </Button>
      </form>
      {create.isError ? <p className="text-xs text-destructive">{errorText(create.error)}</p> : null}
      {revoke.isError ? <p className="text-xs text-destructive">{errorText(revoke.error)}</p> : null}
      {active.map((k) => (
        <div key={k.id} className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="flex-1 truncate">
            <strong className="text-foreground">{k.name}</strong> · {k.prefix}
            {k.builtin ? ' · builtin (rotate via PUBLIC_API_BUILTIN_KEY)' : ''}
            {k.last_used_at ? ` · last used ${new Date(k.last_used_at).toLocaleDateString()}` : ' · never used'}
          </span>
          {!k.builtin ? (
            <Button
              size="sm"
              variant="ghost"
              disabled={revoke.isPending}
              onClick={() => {
                if (confirm(`Revoke the key for “${k.name}”? Whatever uses it stops working.`)) revoke.mutate(k.id);
              }}
            >
              Revoke
            </Button>
          ) : null}
        </div>
      ))}
      {keys.isSuccess && active.length === 0 ? (
        <p className="text-xs text-warning">
          No active keys: the public schedule answers 401 for everyone. Create one here, or set{' '}
          <code>PUBLIC_API_BUILTIN_KEY</code> for the submission site.
        </p>
      ) : null}
    </div>
  );
}

function ChatsCard({ schedule: sc }: { schedule: Schedule }) {
  const qc = useQueryClient();
  const key = ['telegram-chats', sc.edition.id];
  const chats = useQuery({ queryKey: key, queryFn: () => listChats(sc.edition.id) });
  const refresh = () => void qc.invalidateQueries({ queryKey: key });
  const [name, setName] = useState('');
  const [chatId, setChatId] = useState('');
  const add = useMutation({
    mutationFn: () => createChat(sc.edition.id, { name: name.trim(), chat_id: chatId.trim(), enabled: true }),
    onSuccess: () => {
      setName('');
      setChatId('');
      refresh();
    },
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Bot className="h-4 w-4" />
          Telegram
          {sc.telegram_configured ? <Pill tone="success">Bot ready</Pill> : <Pill tone="warning">Dry run</Pill>}
        </CardTitle>
        <CardDescription>
          {sc.telegram_configured
            ? 'Confirmed announcements are posted to the enabled chats below.'
            : 'Set TELEGRAM_BOT_TOKEN on the events backend to post for real. Until then, sending only records the message.'}{' '}
          Add the bot to each channel or group, then enter its chat id (like <code>-1001234567890</code> or <code>@channel</code>).
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        {(chats.data?.items ?? []).map((c) => (
          <ChatRow key={c.id} chat={c} onDone={refresh} />
        ))}
        <form
          className="flex flex-wrap items-center gap-2 border-t border-border pt-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim() && chatId.trim()) add.mutate();
          }}
        >
          <Input aria-label="Chat name" className="min-w-32 flex-1" placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} />
          <Input aria-label="Chat id" className="min-w-32 flex-1" placeholder="Chat id" value={chatId} onChange={(e) => setChatId(e.target.value)} />
          <Button size="sm" type="submit" disabled={!name.trim() || !chatId.trim() || add.isPending}>
            <Plus className="h-3.5 w-3.5" />
            Add chat
          </Button>
          {add.error ? <p className="w-full text-xs text-destructive">{errorText(add.error)}</p> : null}
        </form>
      </CardContent>
    </Card>
  );
}

function ChatRow({ chat, onDone }: { chat: TelegramChat; onDone: () => void }) {
  const toggle = useMutation({
    mutationFn: () => updateChat(chat.id, { name: chat.name, chat_id: chat.chat_id, enabled: !chat.enabled }),
    onSuccess: onDone,
  });
  const del = useMutation({ mutationFn: () => deleteChat(chat.id), onSuccess: onDone });
  return (
    <div className="flex items-center gap-2 text-sm">
      <label className="flex min-w-0 flex-1 items-center gap-2">
        <input type="checkbox" className="accent-primary" checked={chat.enabled} disabled={toggle.isPending} onChange={() => toggle.mutate()} />
        <span className="font-medium">{chat.name}</span>
        <code className="truncate text-xs text-muted-foreground">{chat.chat_id}</code>
      </label>
      <Button
        size="icon"
        variant="ghost"
        aria-label={`Remove ${chat.name}`}
        disabled={del.isPending}
        onClick={() => {
          if (confirm(`Remove the chat “${chat.name}”?`)) del.mutate();
        }}
      >
        <Trash2 className="h-4 w-4" />
      </Button>
    </div>
  );
}
