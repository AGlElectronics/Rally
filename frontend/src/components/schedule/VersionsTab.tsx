import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { History } from 'lucide-react';
import { errorText } from '@/api/client';
import { getVersion, listVersions, restoreVersion, type Schedule, type ScheduleItem, type Version } from '@/api/schedule';
import { formatWhen } from '@/pages/CfPPage';
import { usePutSchedule } from '@/components/schedule/shared';
import { Button } from '@/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { Pill } from '@/components/ui/Pill';
import { EmptyState, ErrorRow, LoadingRow } from '@/components/ui/States';
import { PERMS, useMe } from '@/hooks/useMe';
import { cn } from '@/lib/utils';
import { dayLabel, timeIn, zonedParts } from '@/lib/zonedTime';

const KIND: Record<string, string> = { publish: 'Published', live: 'Live change', restore: 'Restored' };

export function VersionsTab({ schedule: sc }: { schedule: Schedule }) {
  const canEdit = useMe().can(PERMS.scheduleEdit);
  const qc = useQueryClient();
  const put = usePutSchedule(sc.edition.id);
  const versions = useQuery({ queryKey: ['schedule-versions', sc.edition.id, sc.published?.version], queryFn: () => listVersions(sc.edition.id) });
  const [picked, setPicked] = useState<number | null>(null);
  const items = versions.data?.items ?? [];
  const selected = picked ?? items[0]?.version ?? null;
  const detail = useQuery({
    queryKey: ['schedule-version', sc.edition.id, selected],
    queryFn: () => getVersion(sc.edition.id, selected!),
    enabled: selected !== null,
  });
  const [skipped, setSkipped] = useState<string[] | null>(null);
  const restore = useMutation({
    mutationFn: (v: number) => restoreVersion(sc.edition.id, v),
    onSuccess: (res) => {
      put(res.schedule);
      setSkipped(res.skipped);
      void qc.invalidateQueries({ queryKey: ['schedule-versions', sc.edition.id] });
    },
  });

  if (versions.isLoading) return <LoadingRow />;
  if (versions.error) return <ErrorRow message={errorText(versions.error)} />;
  if (items.length === 0) {
    return <EmptyState icon={History} title="No versions yet" description="Each publish and live-day change is kept here." />;
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[20rem_minmax(0,1fr)]">
      <Card className="self-start divide-y divide-border">
        {items.map((v) => (
          <button
            key={v.version}
            type="button"
            onClick={() => setPicked(v.version)}
            className={cn('block w-full px-4 py-3 text-left hover:bg-secondary/60', v.version === selected && 'bg-accent')}
          >
            <div className="flex items-center gap-2">
              <span className="font-semibold">v{v.version}</span>
              <Pill tone={v.kind === 'live' ? 'warning' : v.kind === 'restore' ? 'primary' : 'neutral'}>{KIND[v.kind] ?? v.kind}</Pill>
              {v.is_current ? <Pill tone="success">Public</Pill> : null}
            </div>
            {v.note ? <p className="mt-1 text-sm">{v.note}</p> : null}
            <p className="mt-0.5 text-xs text-muted-foreground">
              {formatWhen(v.published_at)} · {v.published_by || 'someone'} · {v.item_count} items
            </p>
          </button>
        ))}
      </Card>
      <div className="space-y-3">
        {skipped ? (
          <p className="rounded-lg bg-success/10 px-4 py-2 text-sm text-success">
            Loaded into the draft. Check it in Build, then publish.
            {skipped.length > 0 ? ` Skipped: ${skipped.join('; ')}.` : ''}
          </p>
        ) : null}
        {restore.error ? <p className="text-sm text-destructive">{errorText(restore.error)}</p> : null}
        {detail.isLoading ? (
          <LoadingRow />
        ) : detail.data ? (
          <VersionDetail
            v={detail.data}
            draft={sc.items}
            tz={sc.edition.timezone}
            canEdit={canEdit}
            busy={restore.isPending}
            onRestore={() => {
              if (confirm(`Replace the current draft with version ${detail.data!.version}? Unpublished draft changes are lost.`))
                restore.mutate(detail.data!.version);
            }}
          />
        ) : null}
      </div>
    </div>
  );
}

type Change = { kind: 'added' | 'removed' | 'changed'; title: string; what: string[] };

function diff(oldItems: ScheduleItem[], draft: ScheduleItem[], rooms: Map<string, string>, tz: string): Change[] {
  const before = new Map(oldItems.map((i) => [i.id, i]));
  const after = new Map(draft.map((i) => [i.id, i]));
  const when = (i: ScheduleItem) => `${dayLabel(zonedParts(i.starts_at, tz).date)} ${timeIn(i.starts_at, tz)}–${timeIn(i.ends_at, tz)}`;
  const out: Change[] = [];
  for (const i of draft) {
    const o = before.get(i.id);
    if (!o) {
      out.push({ kind: 'added', title: i.title, what: [`${rooms.get(i.room_id) ?? 'room'}, ${when(i)}`] });
      continue;
    }
    const what: string[] = [];
    if (o.title !== i.title) what.push(`renamed from “${o.title}”`);
    if (o.room_id !== i.room_id) what.push(`room ${rooms.get(o.room_id) ?? '?'} → ${rooms.get(i.room_id) ?? '?'}`);
    if (o.starts_at !== i.starts_at || o.ends_at !== i.ends_at) what.push(`${when(o)} → ${when(i)}`);
    if (o.status !== i.status) what.push(`${o.status} → ${i.status}`);
    if (o.staff_only !== i.staff_only) what.push(i.staff_only ? 'now staff only' : 'now public');
    if (o.description !== i.description) what.push('description changed');
    if (what.length > 0) out.push({ kind: 'changed', title: i.title, what });
  }
  for (const o of oldItems) {
    if (!after.has(o.id)) out.push({ kind: 'removed', title: o.title, what: [`was ${rooms.get(o.room_id) ?? 'room'}, ${when(o)}`] });
  }
  return out;
}

function VersionDetail({
  v,
  draft,
  tz,
  canEdit,
  busy,
  onRestore,
}: {
  v: Version;
  draft: ScheduleItem[];
  tz: string;
  canEdit: boolean;
  busy: boolean;
  onRestore: () => void;
}) {
  const rooms = new Map(v.data.rooms.map((r) => [r.id, r.name]));
  const changes = diff(v.data.items, draft, rooms, tz);
  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-3">
        <div>
          <CardTitle>
            Version {v.version} compared with the current draft
          </CardTitle>
          <p className="text-sm text-muted-foreground">
            {changes.length === 0 ? 'The draft is the same as this version.' : `${changes.length} difference${changes.length === 1 ? '' : 's'}.`}
          </p>
        </div>
        {canEdit ? (
          <Button size="sm" variant="secondary" disabled={busy} onClick={onRestore}>
            Load into draft
          </Button>
        ) : null}
      </CardHeader>
      <CardContent>
        <ul className="space-y-1.5 text-sm">
          {changes.map((c, i) => (
            <li key={i} className="flex gap-2">
              <Pill tone={c.kind === 'added' ? 'success' : c.kind === 'removed' ? 'danger' : 'warning'}>
                {c.kind === 'added' ? 'Only in draft' : c.kind === 'removed' ? `Only in v${v.version}` : 'Changed'}
              </Pill>
              <span>
                <span className="font-medium">{c.title}</span>
                <span className="text-muted-foreground"> · {c.what.join('; ')}</span>
              </span>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
