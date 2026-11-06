import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { ArchiveRestore, ArrowDown, ArrowUp, ChevronDown, ChevronRight, Lock, Plus, Trash2 } from 'lucide-react';
import { updateCfP, type CallForPapers, type EventType } from '@/api/cfp';
import { errorText, issuesOf } from '@/api/client';
import { FormFields, textareaClasses } from '@/components/forms/FormFields';
import { Button } from '@/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { Input, Label } from '@/components/ui/Input';
import { Pill, selectClasses } from '@/components/ui/Pill';
import {
  FIELD_TYPES,
  SYSTEM_FIELDS,
  fieldTypeLabel,
  slugifyFieldId,
  type Answers,
  type FieldType,
  type FormField,
} from '@/lib/forms';
import { cn } from '@/lib/utils';

type Draft = FormField & { key: string; saved: boolean };

const toDrafts = (fields: FormField[]): Draft[] => fields.map((f) => ({ ...f, key: f.id, saved: true }));

/**
 * The events lead's form editor. Questions that were ever saved keep their id
 * forever (answers point at it); removing one archives it so old answers stay
 * readable, and it can be restored.
 */
export function FormBuilder({
  cfp,
  eventTypes,
  canEdit,
}: {
  cfp: CallForPapers;
  eventTypes: EventType[];
  canEdit: boolean;
}) {
  const qc = useQueryClient();
  const [fields, setFields] = useState<Draft[]>(() => toDrafts(cfp.form.fields));
  const [open, setOpen] = useState<string | null>(null);
  const [preview, setPreview] = useState<Answers>({});
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (!dirty) setFields(toDrafts(cfp.form.fields));
  }, [cfp.form.fields, dirty]);

  const save = useMutation({
    mutationFn: () =>
      updateCfP(cfp.id, {
        title: cfp.title,
        description: cfp.description,
        status: cfp.status,
        opens_at: cfp.opens_at,
        closes_at: cfp.closes_at,
        form: { fields: fields.map(({ key: _k, saved: _s, ...f }) => f) },
      }),
    onSuccess: (next) => {
      qc.setQueryData(['cfp', cfp.id], next);
      void qc.invalidateQueries({ queryKey: ['cfps'] });
      setFields(toDrafts(next.form.fields));
      setDirty(false);
    },
  });
  const issues = issuesOf(save.error);

  const update = (key: string, patch: Partial<FormField>) => {
    setDirty(true);
    setFields((list) =>
      list.map((f) => {
        if (f.key !== key) return f;
        const next = { ...f, ...patch };
        if (!f.saved && patch.label !== undefined) {
          const taken = new Set(list.filter((o) => o.key !== key).map((o) => o.id));
          next.id = slugifyFieldId(patch.label || 'question', taken);
        }
        return next;
      }),
    );
  };

  const add = (type: FieldType) => {
    const taken = new Set(fields.map((f) => f.id));
    const label = 'New question';
    const key = `new-${Date.now()}`;
    const f: Draft = {
      key,
      saved: false,
      id: slugifyFieldId(label, taken),
      type,
      label,
      required: false,
      ...(type === 'single_choice' || type === 'multi_choice' ? { options: ['Option 1', 'Option 2'] } : {}),
      ...(type === 'file' ? { max_files: 1, max_size_mb: 10, accept: [] } : {}),
    };
    setDirty(true);
    setFields((list) => [...list, f]);
    setOpen(key);
  };

  const move = (key: string, dir: -1 | 1) => {
    setDirty(true);
    setFields((list) => {
      const i = list.findIndex((f) => f.key === key);
      const j = i + dir;
      if (i < 0 || j < 0 || j >= list.length) return list;
      const next = [...list];
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });
  };

  const remove = (f: Draft) => {
    setDirty(true);
    if (f.saved) update(f.key, { archived: true });
    else setFields((list) => list.filter((o) => o.key !== f.key));
    if (open === f.key) setOpen(null);
  };

  const active = fields.filter((f) => !f.archived);
  const archived = fields.filter((f) => f.archived);
  const offered = useMemo(
    () => (cfp.event_type_ids.length ? eventTypes.filter((t) => cfp.event_type_ids.includes(t.id)) : eventTypes),
    [cfp.event_type_ids, eventTypes],
  );

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,26rem)]">
      <div className="space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>Built-in fields</CardTitle>
            <p className="text-sm text-muted-foreground">Always part of the form because scheduling relies on them.</p>
          </CardHeader>
          <CardContent className="space-y-1.5">
            {SYSTEM_FIELDS.map((s) => (
              <div key={s.id} className="flex items-center gap-2 text-sm">
                <Lock className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="font-medium">{s.label}</span>
                <span className="text-muted-foreground">· {s.note}</span>
              </div>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex-row items-center justify-between">
            <CardTitle>Questions</CardTitle>
            {canEdit ? <AddField onAdd={add} /> : null}
          </CardHeader>
          <CardContent className="space-y-2">
            {active.length === 0 ? <p className="text-sm text-muted-foreground">No questions yet. Add the first one.</p> : null}
            {active.map((f, i) => (
              <FieldRow
                key={f.key}
                field={f}
                index={fields.indexOf(f)}
                expanded={open === f.key}
                onToggle={() => setOpen(open === f.key ? null : f.key)}
                onChange={(p) => update(f.key, p)}
                onUp={i > 0 ? () => move(f.key, -1) : undefined}
                onDown={i < active.length - 1 ? () => move(f.key, 1) : undefined}
                onRemove={() => remove(f)}
                issues={issues}
                canEdit={canEdit}
              />
            ))}
            {archived.length > 0 ? (
              <div className="pt-3">
                <p className="mb-1.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Removed questions</p>
                {archived.map((f) => (
                  <div key={f.key} className="flex items-center gap-2 py-1 text-sm text-muted-foreground">
                    <span className="flex-1 truncate line-through">{f.label}</span>
                    {canEdit ? (
                      <Button size="sm" variant="ghost" onClick={() => update(f.key, { archived: false })}>
                        <ArchiveRestore className="h-3.5 w-3.5" />
                        Restore
                      </Button>
                    ) : null}
                  </div>
                ))}
              </div>
            ) : null}
          </CardContent>
        </Card>

        {canEdit ? (
          <div className="sticky bottom-3 z-10 flex items-center gap-3 rounded-card border border-border bg-card p-3 shadow-pop">
            <Button onClick={() => save.mutate()} disabled={!dirty || save.isPending}>
              {save.isPending ? 'Saving…' : 'Save form'}
            </Button>
            {dirty ? (
              <Button
                variant="ghost"
                onClick={() => {
                  setFields(toDrafts(cfp.form.fields));
                  setDirty(false);
                  save.reset();
                }}
              >
                Discard changes
              </Button>
            ) : (
              <span className="text-sm text-muted-foreground">All changes saved.</span>
            )}
            {save.error ? <span className="text-sm text-destructive">{errorText(save.error)}</span> : null}
          </div>
        ) : null}
      </div>

      <div className="lg:sticky lg:top-4 lg:self-start">
        <Card>
          <CardHeader>
            <CardTitle>Preview</CardTitle>
            <p className="text-sm text-muted-foreground">What submitters see. Nothing typed here is saved.</p>
          </CardHeader>
          <CardContent className="space-y-5">
            <div>
              <Label htmlFor="pv-title">
                Title <span className="text-destructive">*</span>
              </Label>
              <Input id="pv-title" placeholder="What is it called?" />
            </div>
            {offered.length > 0 ? (
              <div>
                <Label htmlFor="pv-type">
                  Event type <span className="text-destructive">*</span>
                </Label>
                <select id="pv-type" className={cn(selectClasses, 'w-full')}>
                  {offered.map((t) => (
                    <option key={t.id}>
                      {t.name} ({t.default_minutes} min)
                    </option>
                  ))}
                </select>
              </div>
            ) : null}
            <FormFields
              fields={active}
              answers={preview}
              onChange={(id, v) => setPreview((p) => ({ ...p, [id]: v }))}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function AddField({ onAdd }: { onAdd: (t: FieldType) => void }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="relative">
      <Button size="sm" onClick={() => setOpen((o) => !o)}>
        <Plus className="h-3.5 w-3.5" />
        Add question
      </Button>
      {open ? (
        <>
          <div className="fixed inset-0 z-20" onClick={() => setOpen(false)} />
          <div className="absolute right-0 z-30 mt-1 w-72 rounded-card border border-border bg-card p-1.5 shadow-pop">
            {FIELD_TYPES.map((t) => (
              <button
                key={t.value}
                type="button"
                className="block w-full rounded-md px-3 py-1.5 text-left hover:bg-secondary"
                onClick={() => {
                  onAdd(t.value);
                  setOpen(false);
                }}
              >
                <span className="block text-sm font-medium">{t.label}</span>
                <span className="block text-xs text-muted-foreground">{t.hint}</span>
              </button>
            ))}
          </div>
        </>
      ) : null}
    </div>
  );
}

function FieldRow({
  field,
  index,
  expanded,
  onToggle,
  onChange,
  onUp,
  onDown,
  onRemove,
  issues,
  canEdit,
}: {
  field: Draft;
  index: number;
  expanded: boolean;
  onToggle: () => void;
  onChange: (p: Partial<FormField>) => void;
  onUp?: () => void;
  onDown?: () => void;
  onRemove: () => void;
  issues: Record<string, string>;
  canEdit: boolean;
}) {
  const prefix = `form.fields[${index}]`;
  const rowIssues = Object.entries(issues).filter(([k]) => k.startsWith(prefix));
  const err = (k: string) => issues[`${prefix}.${k}`];
  const num = (v: string) => (v === '' ? undefined : Number(v));

  return (
    <div className={cn('rounded-md border', rowIssues.length ? 'border-destructive' : 'border-border')}>
      <div className="flex items-center gap-2 px-3 py-2">
        <button type="button" className="flex min-w-0 flex-1 items-center gap-2 text-left" onClick={onToggle}>
          {expanded ? <ChevronDown className="h-4 w-4 shrink-0" /> : <ChevronRight className="h-4 w-4 shrink-0" />}
          <span className="truncate text-sm font-medium">{field.label || 'Untitled question'}</span>
          {field.required ? <span className="text-destructive">*</span> : null}
          <Pill className="ml-1">{fieldTypeLabel(field.type)}</Pill>
          {!field.saved ? <Pill tone="primary">new</Pill> : null}
        </button>
        {canEdit ? (
          <div className="flex items-center">
            <IconBtn label="Move up" onClick={onUp}>
              <ArrowUp className="h-3.5 w-3.5" />
            </IconBtn>
            <IconBtn label="Move down" onClick={onDown}>
              <ArrowDown className="h-3.5 w-3.5" />
            </IconBtn>
            <IconBtn label={field.saved ? 'Remove (keeps old answers)' : 'Delete'} onClick={onRemove}>
              <Trash2 className="h-3.5 w-3.5" />
            </IconBtn>
          </div>
        ) : null}
      </div>
      {expanded ? (
        <fieldset disabled={!canEdit} className="space-y-3 border-t border-border px-3 py-3">
          <div className="grid gap-3 sm:grid-cols-[1fr_12rem]">
            <div>
              <Label htmlFor={`${field.key}-label`}>Question</Label>
              <Input id={`${field.key}-label`} value={field.label} onChange={(e) => onChange({ label: e.target.value })} />
              {err('label') ? <p className="mt-1 text-xs text-destructive">{err('label')}</p> : null}
            </div>
            <div>
              <Label htmlFor={`${field.key}-type`}>Type</Label>
              <select
                id={`${field.key}-type`}
                className={cn(selectClasses, 'w-full')}
                value={field.type}
                disabled={field.saved}
                title={field.saved ? 'Saved questions keep their type so existing answers stay valid' : undefined}
                onChange={(e) => onChange({ type: e.target.value as FieldType })}
              >
                {FIELD_TYPES.map((t) => (
                  <option key={t.value} value={t.value}>
                    {t.label}
                  </option>
                ))}
              </select>
            </div>
          </div>
          <div>
            <Label htmlFor={`${field.key}-help`}>Help text</Label>
            <textarea
              id={`${field.key}-help`}
              className={cn(textareaClasses, 'min-h-16')}
              value={field.help ?? ''}
              placeholder="Optional hint shown under the question"
              onChange={(e) => onChange({ help: e.target.value })}
            />
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              className="accent-primary"
              checked={!!field.required}
              onChange={(e) => onChange({ required: e.target.checked })}
            />
            Required to submit
          </label>

          {field.type === 'single_choice' || field.type === 'multi_choice' ? (
            <div>
              <Label htmlFor={`${field.key}-opts`}>Options (one per line)</Label>
              <textarea
                id={`${field.key}-opts`}
                className={cn(textareaClasses, 'min-h-24')}
                value={(field.options ?? []).join('\n')}
                onChange={(e) => onChange({ options: e.target.value.split('\n') })}
              />
              {err('options') ? <p className="mt-1 text-xs text-destructive">{err('options')}</p> : null}
            </div>
          ) : null}

          {['number', 'duration', 'short_text', 'long_text'].includes(field.type) ? (
            <div className="grid grid-cols-2 gap-3 sm:max-w-sm">
              <div>
                <Label htmlFor={`${field.key}-min`}>{field.type.endsWith('text') ? 'Min characters' : 'Minimum'}</Label>
                <Input id={`${field.key}-min`} type="number" value={field.min ?? ''} onChange={(e) => onChange({ min: num(e.target.value) })} />
              </div>
              <div>
                <Label htmlFor={`${field.key}-max`}>{field.type.endsWith('text') ? 'Max characters' : 'Maximum'}</Label>
                <Input id={`${field.key}-max`} type="number" value={field.max ?? ''} onChange={(e) => onChange({ max: num(e.target.value) })} />
                {err('max') ? <p className="mt-1 text-xs text-destructive">{err('max')}</p> : null}
              </div>
            </div>
          ) : null}

          {field.type === 'file' ? (
            <div className="grid gap-3 sm:grid-cols-3">
              <div className="sm:col-span-3">
                <Label htmlFor={`${field.key}-accept`}>Allowed types</Label>
                <Input
                  id={`${field.key}-accept`}
                  value={(field.accept ?? []).join(', ')}
                  placeholder="any, or e.g. image/*, application/pdf"
                  onChange={(e) =>
                    onChange({
                      accept: e.target.value
                        .split(',')
                        .map((s) => s.trim())
                        .filter(Boolean),
                    })
                  }
                />
              </div>
              <div>
                <Label htmlFor={`${field.key}-files`}>Max files</Label>
                <Input
                  id={`${field.key}-files`}
                  type="number"
                  min={1}
                  max={10}
                  value={field.max_files ?? 1}
                  onChange={(e) => onChange({ max_files: Number(e.target.value) || 1 })}
                />
                {err('max_files') ? <p className="mt-1 text-xs text-destructive">{err('max_files')}</p> : null}
              </div>
              <div>
                <Label htmlFor={`${field.key}-size`}>Max size (MB)</Label>
                <Input
                  id={`${field.key}-size`}
                  type="number"
                  min={1}
                  max={100}
                  value={field.max_size_mb ?? 10}
                  onChange={(e) => onChange({ max_size_mb: Number(e.target.value) || 10 })}
                />
                {err('max_size_mb') ? <p className="mt-1 text-xs text-destructive">{err('max_size_mb')}</p> : null}
              </div>
            </div>
          ) : null}

          <p className="text-xs text-muted-foreground">
            Field id <code>{field.id}</code>
            {field.saved ? ' (fixed)' : ' (set from the question until saved)'}
            {err('id') ? <span className="text-destructive"> · {err('id')}</span> : null}
          </p>
        </fieldset>
      ) : null}
    </div>
  );
}

function IconBtn({ label, onClick, children }: { label: string; onClick?: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      disabled={!onClick}
      onClick={onClick}
      className="rounded-pill p-1.5 text-muted-foreground hover:bg-secondary hover:text-foreground disabled:opacity-30 disabled:hover:bg-transparent"
    >
      {children}
    </button>
  );
}