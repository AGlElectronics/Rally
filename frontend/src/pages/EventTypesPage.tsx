import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus, Shapes, Trash2 } from 'lucide-react';
import {
  createEventType,
  deleteEventType,
  listEventTypes,
  updateEventType,
  type EventType,
  type EventTypeInput,
} from '@/api/cfp';
import { errorText } from '@/api/client';
import { listRooms, type Room } from '@/api/schedule';
import { cn } from '@/lib/utils';
import { EditionPicker } from '@/components/EditionPicker';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { PageContainer, PageHeader } from '@/components/ui/PageHeader';
import { EmptyState, ErrorRow, LoadingRow } from '@/components/ui/States';
import { useEdition } from '@/hooks/useEdition';
import { PERMS, useMe } from '@/hooks/useMe';

const PALETTE = ['#6366f1', '#ec4899', '#f59e0b', '#10b981', '#0ea5e9', '#8b5cf6', '#ef4444', '#64748b'];

export function EventTypesPage() {
  const { editions, edition, setEditionId, isLoading, error } = useEdition();
  const canEdit = useMe().can(PERMS.cfpManage);
  const qc = useQueryClient();
  const types = useQuery({
    queryKey: ['event-types', edition?.id],
    queryFn: () => listEventTypes(edition!.id),
    enabled: !!edition,
  });
  const rooms = useQuery({
    queryKey: ['rooms', edition?.id],
    queryFn: () => listRooms(edition!.id),
    enabled: !!edition,
  });
  const items = types.data?.items ?? [];
  const roomList = rooms.data?.items ?? [];
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['event-types'] });
    void qc.invalidateQueries({ queryKey: ['schedule'] });
  };

  const create = useMutation({
    mutationFn: () =>
      createEventType(edition!.id, {
        name: `New type ${items.length + 1}`,
        color: PALETTE[items.length % PALETTE.length],
        default_minutes: 60,
        gap_after_minutes: 0,
        sort_order: items.length,
      }),
    onSuccess: refresh,
  });

  return (
    <PageContainer>
      <PageHeader
        title="Event types"
        description="Kinds of programme items (panel, workshop, stage show…). Their length and the gap after them drive the schedule rules."
        actions={
          <>
            <EditionPicker editions={editions} edition={edition} onChange={setEditionId} />
            {canEdit && edition ? (
              <Button onClick={() => create.mutate()} disabled={create.isPending}>
                <Plus className="h-4 w-4" />
                Add type
              </Button>
            ) : null}
          </>
        }
      />
      {create.error ? <p className="mb-3 text-sm text-destructive">{errorText(create.error)}</p> : null}
      {isLoading || types.isLoading ? (
        <LoadingRow />
      ) : error || types.error ? (
        <ErrorRow message={errorText(error || types.error)} />
      ) : items.length === 0 ? (
        <EmptyState icon={Shapes} title="No event types yet" description="Add the kinds of events this edition runs." />
      ) : (
        <Card className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="text-left text-xs uppercase tracking-wide text-muted-foreground">
              <tr className="border-b border-border">
                <th className="px-4 py-2 font-semibold">Name</th>
                <th className="px-2 py-2 font-semibold">Default length</th>
                <th className="px-2 py-2 font-semibold">Gap after</th>
                <th className="px-2 py-2 font-semibold">Rooms</th>
                <th className="w-px px-4 py-2" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {items.map((t) => (
                <TypeRow key={t.id} type={t} rooms={roomList} canEdit={canEdit} onChanged={refresh} />
              ))}
            </tbody>
          </table>
        </Card>
      )}
    </PageContainer>
  );
}

function TypeRow({
  type,
  rooms,
  canEdit,
  onChanged,
}: {
  type: EventType;
  rooms: Room[];
  canEdit: boolean;
  onChanged: () => void;
}) {
  const [draft, setDraft] = useState<EventTypeInput>(() => ({
    name: type.name,
    color: type.color,
    default_minutes: type.default_minutes,
    gap_after_minutes: type.gap_after_minutes,
    sort_order: type.sort_order,
    room_ids: type.room_ids ?? [],
  }));
  const roomIds = draft.room_ids ?? [];
  const dirty =
    draft.name !== type.name ||
    draft.color !== type.color ||
    draft.default_minutes !== type.default_minutes ||
    draft.gap_after_minutes !== type.gap_after_minutes ||
    [...roomIds].sort().join() !== [...(type.room_ids ?? [])].sort().join();
  const save = useMutation({ mutationFn: () => updateEventType(type.id, draft), onSuccess: onChanged });
  const remove = useMutation({ mutationFn: () => deleteEventType(type.id), onSuccess: onChanged });
  const set = (p: Partial<EventTypeInput>) => setDraft((d) => ({ ...d, ...p }));
  const err = save.error || remove.error;

  return (
    <tr className="align-top">
      <td className="px-4 py-2">
        <div className="flex items-center gap-2">
          <input
            type="color"
            aria-label="Colour"
            disabled={!canEdit}
            value={draft.color}
            onChange={(e) => set({ color: e.target.value })}
            className="h-8 w-8 shrink-0 cursor-pointer rounded-md border border-border bg-transparent p-0.5"
          />
          <Input disabled={!canEdit} value={draft.name} onChange={(e) => set({ name: e.target.value })} className="min-w-40" />
        </div>
        {err ? <p className="mt-1 text-xs text-destructive">{errorText(err)}</p> : null}
      </td>
      <td className="px-2 py-2">
        <MinutesInput disabled={!canEdit} value={draft.default_minutes} onChange={(v) => set({ default_minutes: v })} />
      </td>
      <td className="px-2 py-2">
        <MinutesInput disabled={!canEdit} value={draft.gap_after_minutes} onChange={(v) => set({ gap_after_minutes: v })} />
      </td>
      <td className="px-2 py-2">
        {rooms.length === 0 ? (
          <span className="text-xs text-muted-foreground">Any room</span>
        ) : (
          <div className="flex max-w-xs flex-wrap gap-1" title="None picked means any room">
            {rooms.map((r) => {
              const on = roomIds.includes(r.id);
              return (
                <button
                  key={r.id}
                  type="button"
                  disabled={!canEdit}
                  aria-pressed={on}
                  onClick={() => set({ room_ids: on ? roomIds.filter((x) => x !== r.id) : [...roomIds, r.id] })}
                  className={cn(
                    'rounded-pill border px-2 py-0.5 text-xs font-medium transition-colors',
                    on ? 'border-primary bg-primary-soft text-primary' : 'border-border text-muted-foreground hover:bg-secondary'
                  )}
                >
                  {r.name}
                </button>
              );
            })}
            {roomIds.length === 0 ? <span className="self-center text-[11px] text-muted-foreground">(any)</span> : null}
          </div>
        )}
      </td>
      <td className="px-4 py-2">
        {canEdit ? (
          <div className="flex items-center gap-1">
            <Button size="sm" disabled={!dirty || save.isPending} onClick={() => save.mutate()}>
              Save
            </Button>
            <Button
              size="icon"
              variant="ghost"
              aria-label="Delete"
              title="Delete"
              disabled={remove.isPending}
              onClick={() => {
                if (confirm(`Delete "${type.name}"?`)) remove.mutate();
              }}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ) : null}
      </td>
    </tr>
  );
}

function MinutesInput({ value, onChange, disabled }: { value: number; onChange: (v: number) => void; disabled?: boolean }) {
  return (
    <div className="flex items-center gap-1.5">
      <Input
        type="number"
        min={0}
        step={5}
        disabled={disabled}
        className="w-24"
        value={value}
        onChange={(e) => onChange(Number(e.target.value) || 0)}
      />
      <span className="text-xs text-muted-foreground">min</span>
    </div>
  );
}
