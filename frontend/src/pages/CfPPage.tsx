import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useNavigate } from '@tanstack/react-router';
import { CalendarRange, ChevronRight, Plus } from 'lucide-react';
import { createCfP, listCfPs, listSubmissions, type CallForPapers } from '@/api/cfp';
import { errorText } from '@/api/client';
import { EditionPicker } from '@/components/EditionPicker';
import { Button } from '@/components/ui/Button';
import { Card, CardContent } from '@/components/ui/Card';
import { Input, Label } from '@/components/ui/Input';
import { PageContainer, PageHeader } from '@/components/ui/PageHeader';
import { Pill, type Tone } from '@/components/ui/Pill';
import { EmptyState, ErrorRow, LoadingRow } from '@/components/ui/States';
import { useEdition } from '@/hooks/useEdition';
import { PERMS, useMe } from '@/hooks/useMe';

const STATUS_TONE: Record<CallForPapers['status'], Tone> = { draft: 'neutral', open: 'success', closed: 'warning' };

export function formatWhen(iso?: string | null): string {
  return iso ? new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : '';
}

export function CfPPage() {
  const { editions, edition, setEditionId, isLoading, error } = useEdition();
  const { can } = useMe();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState('');

  const cfps = useQuery({
    queryKey: ['cfps', edition?.id],
    queryFn: () => listCfPs(edition!.id),
    enabled: !!edition,
  });
  const subs = useQuery({
    queryKey: ['submissions', edition?.id, 'all'],
    queryFn: () => listSubmissions(edition!.id),
    enabled: !!edition && can(PERMS.submissionReview),
  });

  const create = useMutation({
    mutationFn: () => createCfP(edition!.id, { title: title.trim(), description: '', status: 'draft' }),
    onSuccess: (c) => {
      void qc.invalidateQueries({ queryKey: ['cfps'] });
      void navigate({ to: '/cfp/$cfpId', params: { cfpId: c.id }, search: { tab: 'form' } });
    },
  });

  const counts = (cfpId: string) => {
    const items = (subs.data?.items ?? []).filter((s) => s.cfp_id === cfpId && s.state !== 'draft');
    return { total: items.length, waiting: items.filter((s) => s.state === 'submitted').length };
  };

  return (
    <PageContainer>
      <PageHeader
        title="Calls for papers"
        description={edition ? `Submission calls for ${edition.title}.` : undefined}
        actions={
          <>
            <EditionPicker editions={editions} edition={edition} onChange={setEditionId} />
            {can(PERMS.cfpManage) && edition ? (
              <Button onClick={() => setCreating(true)}>
                <Plus className="h-4 w-4" />
                New call
              </Button>
            ) : null}
          </>
        }
      />

      {creating ? (
        <Card className="mb-4">
          <CardContent>
            <form
              className="flex flex-wrap items-end gap-3"
              onSubmit={(e) => {
                e.preventDefault();
                if (title.trim()) create.mutate();
              }}
            >
              <div className="min-w-64 flex-1">
                <Label htmlFor="new-cfp">Title</Label>
                <Input
                  id="new-cfp"
                  autoFocus
                  value={title}
                  placeholder="e.g. Panels & workshops"
                  onChange={(e) => setTitle(e.target.value)}
                />
              </div>
              <Button type="submit" disabled={!title.trim() || create.isPending}>
                Create as draft
              </Button>
              <Button type="button" variant="ghost" onClick={() => setCreating(false)}>
                Cancel
              </Button>
            </form>
            {create.error ? <p className="mt-2 text-sm text-destructive">{errorText(create.error)}</p> : null}
          </CardContent>
        </Card>
      ) : null}

      {isLoading || cfps.isLoading ? (
        <LoadingRow />
      ) : error || cfps.error ? (
        <ErrorRow message={errorText(error || cfps.error)} />
      ) : !edition ? (
        <EmptyState icon={CalendarRange} title="No editions yet" description="Create an edition first, then open calls on it." />
      ) : (cfps.data?.items ?? []).length === 0 ? (
        <EmptyState
          icon={CalendarRange}
          title="No calls yet"
          description="A call collects submissions through a form you design: panels, workshops, performances, anything."
        />
      ) : (
        <div className="grid gap-3">
          {cfps.data!.items.map((c) => {
            const n = counts(c.id);
            return (
              <Link key={c.id} to="/cfp/$cfpId" params={{ cfpId: c.id }} search={{ tab: 'submissions' }}>
                <Card clickable>
                  <CardContent className="flex items-center gap-4">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-heading font-bold">{c.title}</span>
                        <Pill tone={STATUS_TONE[c.status]}>{c.status}</Pill>
                        {c.review_mode === 'scored' ? <Pill>scored review</Pill> : null}
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {c.form.fields.filter((f) => !f.archived).length} questions
                        {c.closes_at ? ` · closes ${formatWhen(c.closes_at)}` : ''}
                        {subs.data ? ` · ${n.total} submitted, ${n.waiting} awaiting a decision` : ''}
                      </p>
                    </div>
                    <ChevronRight className="h-5 w-5 text-muted-foreground" />
                  </CardContent>
                </Card>
              </Link>
            );
          })}
        </div>
      )}
    </PageContainer>
  );
}
