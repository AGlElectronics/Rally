import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { PageHeader } from '@/components/ui/PageHeader';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Input, Label } from '@/components/ui/Input';
import {
  acceptSubmission,
  createCfP,
  createSubmission,
  listCfPs,
  listEditions,
  listSubmissions,
  rejectSubmission,
  submitSubmission,
} from '@/api/cfp';
import { ApiError } from '@/api/client';

export function CfPPage() {
  const qc = useQueryClient();
  const editions = useQuery({ queryKey: ['editions'], queryFn: listEditions });
  const [editionId, setEditionId] = useState<string>('');
  const activeEdition =
    editionId || editions.data?.items[0]?.id || '00000000-0000-0000-0000-000000000030';

  const cfps = useQuery({
    queryKey: ['cfps', activeEdition],
    queryFn: () => listCfPs(activeEdition),
    enabled: !!activeEdition,
  });
  const openCfp = cfps.data?.items.find((c) => c.status === 'open') ?? cfps.data?.items[0];

  const subs = useQuery({
    queryKey: ['subs', activeEdition, openCfp?.id],
    queryFn: () => listSubmissions(activeEdition, { cfp_id: openCfp?.id }),
    enabled: !!activeEdition && !!openCfp?.id,
  });

  const [title, setTitle] = useState('Demo CfP');
  const [talkTitle, setTalkTitle] = useState('');
  const [abstract, setAbstract] = useState('');
  const [speaker, setSpeaker] = useState('');
  const [error, setError] = useState<string | null>(null);

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['cfps'] });
    void qc.invalidateQueries({ queryKey: ['subs'] });
  };

  const openMutation = useMutation({
    mutationFn: () =>
      createCfP(activeEdition, { title, description: 'Open for talk proposals', status: 'open' }),
    onSuccess: () => {
      setError(null);
      invalidate();
    },
    onError: (e: unknown) => setError(e instanceof ApiError ? e.message : 'Failed to open CfP'),
  });

  const createTalk = useMutation({
    mutationFn: async () => {
      if (!openCfp) throw new Error('No open CfP');
      const s = await createSubmission(openCfp.id, {
        title: talkTitle,
        abstract,
        speakers: [{ name: speaker || 'Speaker' }],
      });
      return submitSubmission(s.id);
    },
    onSuccess: () => {
      setTalkTitle('');
      setAbstract('');
      setSpeaker('');
      setError(null);
      invalidate();
    },
    onError: (e: unknown) => setError(e instanceof ApiError ? e.message : 'Submit failed'),
  });

  const decide = useMutation({
    mutationFn: async ({ id, ok }: { id: string; ok: boolean }) =>
      ok ? acceptSubmission(id, 'Accepted') : rejectSubmission(id, 'Not a fit'),
    onSuccess: () => invalidate(),
    onError: (e: unknown) => setError(e instanceof ApiError ? e.message : 'Decision failed'),
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title="Call for papers"
        description="Open a CfP on an edition, submit a talk, and accept or reject proposals."
      />

      {error ? <p className="text-sm text-destructive">{error}</p> : null}

      <Card className="space-y-3 p-4">
        <Label>Edition</Label>
        <select
          className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
          value={activeEdition}
          onChange={(e) => setEditionId(e.target.value)}
        >
          {(editions.data?.items ?? []).map((ed) => (
            <option key={ed.id} value={ed.id}>
              {ed.title}
            </option>
          ))}
        </select>
        {!openCfp ? (
          <div className="flex flex-wrap items-end gap-3">
            <div className="min-w-[12rem] flex-1">
              <Label htmlFor="cfp-title">CfP title</Label>
              <Input id="cfp-title" value={title} onChange={(e) => setTitle(e.target.value)} />
            </div>
            <Button onClick={() => openMutation.mutate()} disabled={openMutation.isPending}>
              Open CfP
            </Button>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            Active CfP: <span className="font-medium text-foreground">{openCfp.title}</span> (
            {openCfp.status})
          </p>
        )}
      </Card>

      {openCfp ? (
        <Card className="space-y-3 p-4">
          <h2 className="font-heading text-lg font-semibold">New submission</h2>
          <div>
            <Label htmlFor="talk-title">Talk title</Label>
            <Input
              id="talk-title"
              value={talkTitle}
              onChange={(e) => setTalkTitle(e.target.value)}
            />
          </div>
          <div>
            <Label htmlFor="speaker">Speaker</Label>
            <Input id="speaker" value={speaker} onChange={(e) => setSpeaker(e.target.value)} />
          </div>
          <div>
            <Label htmlFor="abstract">Abstract</Label>
            <textarea
              id="abstract"
              className="min-h-24 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
              value={abstract}
              onChange={(e) => setAbstract(e.target.value)}
            />
          </div>
          <Button
            onClick={() => createTalk.mutate()}
            disabled={!talkTitle || !abstract || createTalk.isPending}
          >
            Submit talk
          </Button>
        </Card>
      ) : null}

      <Card className="space-y-2 p-4">
        <h2 className="font-heading text-lg font-semibold">Submissions</h2>
        {(subs.data?.items ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">No submissions yet.</p>
        ) : (
          <ul className="divide-y divide-border">
            {(subs.data?.items ?? []).map((s) => (
              <li key={s.id} className="flex flex-wrap items-center justify-between gap-2 py-3">
                <div>
                  <div className="font-medium">{s.title}</div>
                  <div className="text-xs text-muted-foreground">{s.state}</div>
                </div>
                {s.state === 'submitted' ? (
                  <div className="flex gap-2">
                    <Button size="sm" onClick={() => decide.mutate({ id: s.id, ok: true })}>
                      Accept
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => decide.mutate({ id: s.id, ok: false })}
                    >
                      Reject
                    </Button>
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
