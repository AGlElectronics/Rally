import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { updateCfP, type CallForPapers, type CfPStatus, type EventType, type ReviewMode } from '@/api/cfp';
import { errorText, issuesOf } from '@/api/client';
import { textareaClasses } from '@/components/forms/FormFields';
import { Button } from '@/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { Input, Label } from '@/components/ui/Input';
import { ColorDot, selectClasses } from '@/components/ui/Pill';
import { DEFAULT_STATUS_LABELS, SUBMISSION_STATES } from '@/lib/forms';
import { cn } from '@/lib/utils';

function toLocal(iso?: string | null): string {
  if (!iso) return '';
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
const fromLocal = (v: string) => (v ? new Date(v).toISOString() : null);

export function CfPSettings({ cfp, eventTypes, canEdit }: { cfp: CallForPapers; eventTypes: EventType[]; canEdit: boolean }) {
  const qc = useQueryClient();
  const [title, setTitle] = useState(cfp.title);
  const [description, setDescription] = useState(cfp.description);
  const [status, setStatus] = useState<CfPStatus>(cfp.status);
  const [opensAt, setOpensAt] = useState(toLocal(cfp.opens_at));
  const [closesAt, setClosesAt] = useState(toLocal(cfp.closes_at));
  const [reviewMode, setReviewMode] = useState<ReviewMode>(cfp.review_mode);
  const [typeIds, setTypeIds] = useState<string[]>(cfp.event_type_ids);
  const [labels, setLabels] = useState<Record<string, string>>(cfp.status_labels ?? {});

  const save = useMutation({
    mutationFn: () =>
      updateCfP(cfp.id, {
        title,
        description,
        status,
        opens_at: fromLocal(opensAt),
        closes_at: fromLocal(closesAt),
        review_mode: reviewMode,
        event_type_ids: typeIds,
        status_labels: Object.fromEntries(Object.entries(labels).filter(([, v]) => v.trim())),
      }),
    onSuccess: (next) => {
      qc.setQueryData(['cfp', cfp.id], next);
      void qc.invalidateQueries({ queryKey: ['cfps'] });
    },
  });
  const issues = issuesOf(save.error);
  const err = (k: string) => (issues[k] ? <p className="mt-1 text-xs text-destructive">{issues[k]}</p> : null);

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <fieldset disabled={!canEdit} className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>The call</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <Label htmlFor="s-title">Title</Label>
              <Input id="s-title" value={title} onChange={(e) => setTitle(e.target.value)} />
              {err('title')}
            </div>
            <div>
              <Label htmlFor="s-desc">Introduction for submitters</Label>
              <textarea
                id="s-desc"
                className={cn(textareaClasses, 'min-h-32')}
                value={description}
                placeholder="What you're looking for, deadlines, what happens after submitting…"
                onChange={(e) => setDescription(e.target.value)}
              />
              {err('description')}
            </div>
            <div className="grid gap-3 sm:grid-cols-3">
              <div>
                <Label htmlFor="s-status">Status</Label>
                <select
                  id="s-status"
                  className={cn(selectClasses, 'w-full')}
                  value={status}
                  onChange={(e) => setStatus(e.target.value as CfPStatus)}
                >
                  <option value="draft">Draft (hidden)</option>
                  <option value="open">Open</option>
                  <option value="closed">Closed</option>
                </select>
              </div>
              <div>
                <Label htmlFor="s-open">Opens</Label>
                <Input id="s-open" type="datetime-local" value={opensAt} onChange={(e) => setOpensAt(e.target.value)} />
              </div>
              <div>
                <Label htmlFor="s-close">Closes</Label>
                <Input id="s-close" type="datetime-local" value={closesAt} onChange={(e) => setClosesAt(e.target.value)} />
                {err('closes_at')}
              </div>
            </div>
            <p className="text-xs text-muted-foreground">
              An open call accepts submissions between the two dates (either can be empty). Submitters can keep editing until it
              closes.
            </p>
          </CardContent>
        </Card>

        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Review</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              {(
                [
                  ['lead', 'The lead decides', 'Reviewers read and comment; whoever can decide accepts or rejects.'],
                  ['scored', 'Scored', 'Reviewers also give each submission 1 to 5 stars; the average helps the lead decide.'],
                ] as const
              ).map(([value, label, hint]) => (
                <label key={value} className="flex items-start gap-2.5 rounded-md border border-border p-3 text-sm">
                  <input
                    type="radio"
                    name="review-mode"
                    className="mt-0.5 accent-primary"
                    checked={reviewMode === value}
                    onChange={() => setReviewMode(value)}
                  />
                  <span>
                    <span className="block font-medium">{label}</span>
                    <span className="block text-muted-foreground">{hint}</span>
                  </span>
                </label>
              ))}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Event types offered</CardTitle>
              <p className="text-sm text-muted-foreground">
                Submitters pick one. Leave all unticked to offer every type of the edition.
              </p>
            </CardHeader>
            <CardContent className="space-y-1.5">
              {eventTypes.length === 0 ? (
                <p className="text-sm text-muted-foreground">No event types yet; the type question is skipped.</p>
              ) : (
                eventTypes.map((t) => (
                  <label key={t.id} className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      className="accent-primary"
                      checked={typeIds.includes(t.id)}
                      onChange={(e) => setTypeIds((ids) => (e.target.checked ? [...ids, t.id] : ids.filter((i) => i !== t.id)))}
                    />
                    <ColorDot color={t.color} />
                    {t.name}
                    <span className="text-muted-foreground">· {t.default_minutes} min</span>
                  </label>
                ))
              )}
              {err('event_type_ids')}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Status names shown to submitters</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-3 sm:grid-cols-2">
              {SUBMISSION_STATES.map((s) => (
                <div key={s}>
                  <Label htmlFor={`lbl-${s}`} className="capitalize">
                    {s}
                  </Label>
                  <Input
                    id={`lbl-${s}`}
                    value={labels[s] ?? ''}
                    placeholder={DEFAULT_STATUS_LABELS[s]}
                    onChange={(e) => setLabels((l) => ({ ...l, [s]: e.target.value }))}
                  />
                  {err(`status_labels.${s}`)}
                </div>
              ))}
            </CardContent>
          </Card>
        </div>
      </fieldset>
      {canEdit ? (
        <div className="mt-6 flex items-center gap-3">
          <Button type="submit" disabled={save.isPending}>
            {save.isPending ? 'Saving…' : 'Save settings'}
          </Button>
          {save.isSuccess ? <span className="text-sm text-success">Saved.</span> : null}
          {save.error ? <span className="text-sm text-destructive">{errorText(save.error)}</span> : null}
        </div>
      ) : null}
    </form>
  );
}
