import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useParams } from '@tanstack/react-router';
import { ArrowLeft, Check, Star, X } from 'lucide-react';
import {
  API,
  acceptSubmission,
  addNote,
  getCfP,
  getScores,
  getSubmission,
  listNotes,
  rejectSubmission,
  setScore,
  type Submission,
} from '@/api/cfp';
import { errorText } from '@/api/client';
import { AnswersView } from '@/components/forms/AnswersView';
import { textareaClasses } from '@/components/forms/FormFields';
import { Button } from '@/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { PageContainer } from '@/components/ui/PageHeader';
import { Pill, StatePill } from '@/components/ui/Pill';
import { ErrorRow, LoadingRow } from '@/components/ui/States';
import { PERMS, useMe } from '@/hooks/useMe';
import { cn } from '@/lib/utils';
import { openFile } from '@/lib/upload';
import { formatWhen } from '@/pages/CfPPage';

export function SubmissionReviewPage() {
  const { submissionId } = useParams({ from: '/app/submissions/$submissionId' });
  const sub = useQuery({ queryKey: ['submission', submissionId], queryFn: () => getSubmission(submissionId) });
  const cfp = useQuery({
    queryKey: ['cfp', sub.data?.cfp_id],
    queryFn: () => getCfP(sub.data!.cfp_id),
    enabled: !!sub.data,
  });
  const { can, me } = useMe();

  if (sub.isLoading) return <LoadingRow />;
  if (sub.error || !sub.data) return <ErrorRow message={errorText(sub.error) || 'Submission not found.'} />;
  const s = sub.data;
  const fields = cfp.data?.form.fields ?? [];
  const open = (id: string) => void openFile(API, id).catch((e) => alert(errorText(e)));

  return (
    <PageContainer>
      <Link
        to="/cfp/$cfpId"
        params={{ cfpId: s.cfp_id }}
        search={{ tab: 'submissions' }}
        className="mb-3 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        {s.cfp_title || 'Back to the call'}
      </Link>

      <div className="mb-6 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-heading text-2xl font-extrabold leading-tight tracking-tight">{s.title}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            by {s.submitter_name}
            {s.submitted_at ? ` · submitted ${formatWhen(s.submitted_at)}` : ''}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {s.event_type_name ? <Pill>{s.event_type_name}</Pill> : null}
          {s.duration_minutes ? <Pill>{s.duration_minutes} min</Pill> : null}
          <StatePill state={s.state} />
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,24rem)]">
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Answers</CardTitle>
            </CardHeader>
            <CardContent>
              {cfp.isLoading ? <LoadingRow /> : <AnswersView fields={fields} answers={s.answers ?? {}} files={s.files ?? []} onOpenFile={open} />}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>People</CardTitle>
            </CardHeader>
            <CardContent>
              <ul className="space-y-1.5 text-sm">
                {(s.people ?? []).map((p) => (
                  <li key={p.id} className="flex items-center gap-2">
                    <span className="font-medium">{p.display_name || p.invite_email || 'Invited presenter'}</span>
                    <span className="text-muted-foreground">{p.role === 'owner' ? 'submitter' : 'co-presenter'}</span>
                    {p.status === 'invited' ? <Pill tone="warning">invite pending</Pill> : null}
                  </li>
                ))}
              </ul>
            </CardContent>
          </Card>
        </div>

        <div className="space-y-6">
          <Decision sub={s} canDecide={can(PERMS.submissionDecide)} />
          {cfp.data?.review_mode === 'scored' ? (
            <Scores sub={s} canScore={can(PERMS.submissionReview) && s.submitter_user_id !== me?.id} />
          ) : null}
          <Notes submissionId={s.id} canWrite={can(PERMS.submissionReview)} />
        </div>
      </div>
    </PageContainer>
  );
}

function Decision({ sub, canDecide }: { sub: Submission; canDecide: boolean }) {
  const qc = useQueryClient();
  const [note, setNote] = useState('');
  const decide = useMutation({
    mutationFn: (ok: boolean) => (ok ? acceptSubmission(sub.id, note) : rejectSubmission(sub.id, note)),
    onSuccess: (next) => {
      qc.setQueryData(['submission', sub.id], next);
      void qc.invalidateQueries({ queryKey: ['submissions'] });
      setNote('');
    },
  });

  if (sub.state !== 'submitted') {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Decision</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          {sub.state === 'accepted' || sub.state === 'rejected' ? (
            <>
              <p>
                {sub.state === 'accepted' ? 'Accepted' : 'Not accepted'}
                {sub.decided_at ? ` on ${formatWhen(sub.decided_at)}` : ''}.
              </p>
              {sub.decision_note ? (
                <blockquote className="whitespace-pre-wrap border-l-2 border-border pl-3 text-muted-foreground">{sub.decision_note}</blockquote>
              ) : null}
            </>
          ) : (
            <p className="text-muted-foreground">
              {sub.state === 'draft' ? 'Still a draft; nothing to decide yet.' : 'Withdrawn by the submitter.'}
            </p>
          )}
        </CardContent>
      </Card>
    );
  }
  if (!canDecide) {
    return (
      <Card>
        <CardContent className="text-sm text-muted-foreground">Awaiting a decision from the events lead.</CardContent>
      </Card>
    );
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Decision</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <textarea
          className={cn(textareaClasses, 'min-h-24')}
          placeholder="Message to the submitter (optional, they will see this)"
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
        <div className="flex gap-2">
          <Button onClick={() => decide.mutate(true)} disabled={decide.isPending}>
            <Check className="h-4 w-4" />
            Accept
          </Button>
          <Button variant="secondary" onClick={() => decide.mutate(false)} disabled={decide.isPending}>
            <X className="h-4 w-4" />
            Reject
          </Button>
        </div>
        {decide.error ? <p className="text-sm text-destructive">{errorText(decide.error)}</p> : null}
      </CardContent>
    </Card>
  );
}

function Scores({ sub, canScore }: { sub: Submission; canScore: boolean }) {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ['scores', sub.id], queryFn: () => getScores(sub.id) });
  const [note, setNote] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: ({ score, text }: { score: number; text: string }) => setScore(sub.id, score, text),
    onSuccess: (next) => {
      qc.setQueryData(['scores', sub.id], next);
      void qc.invalidateQueries({ queryKey: ['submissions'] });
    },
  });
  const mine = q.data?.mine;
  const noteValue = note ?? mine?.note ?? '';

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between">
        <CardTitle>Scores</CardTitle>
        {q.data?.average != null ? (
          <span className="text-sm font-semibold tabular-nums">
            ⌀ {q.data.average.toFixed(1)} <span className="font-normal text-muted-foreground">({q.data.scores.length})</span>
          </span>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-4">
        {canScore && sub.state === 'submitted' ? (
          <div className="space-y-2">
            <div className="flex items-center gap-1" role="radiogroup" aria-label="Your score">
              {[1, 2, 3, 4, 5].map((n) => (
                <button
                  key={n}
                  type="button"
                  aria-label={`${n} of 5`}
                  onClick={() => save.mutate({ score: n, text: noteValue })}
                  className="rounded p-0.5 text-warning hover:scale-110"
                >
                  <Star className={cn('h-6 w-6', mine && n <= mine.score ? 'fill-current' : 'opacity-40')} />
                </button>
              ))}
              <span className="ml-2 text-xs text-muted-foreground">{mine ? 'Your score' : 'Not scored yet'}</span>
            </div>
            <textarea
              className={cn(textareaClasses, 'min-h-16')}
              placeholder="Why this score? (only staff see this)"
              value={noteValue}
              onChange={(e) => setNote(e.target.value)}
              onBlur={() => {
                if (mine && note !== null && note !== mine.note) save.mutate({ score: mine.score, text: note });
              }}
            />
            {save.error ? <p className="text-sm text-destructive">{errorText(save.error)}</p> : null}
          </div>
        ) : null}
        {q.isLoading ? (
          <LoadingRow />
        ) : (q.data?.scores ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">No scores yet.</p>
        ) : (
          <ul className="space-y-2 text-sm">
            {q.data!.scores.map((sc) => (
              <li key={sc.reviewer_id}>
                <div className="flex items-center gap-2">
                  <span className="font-medium">{sc.reviewer_name}</span>
                  <span className="text-warning">{'★'.repeat(sc.score)}</span>
                </div>
                {sc.note ? <p className="whitespace-pre-wrap text-muted-foreground">{sc.note}</p> : null}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

function Notes({ submissionId, canWrite }: { submissionId: string; canWrite: boolean }) {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ['notes', submissionId], queryFn: () => listNotes(submissionId) });
  const [body, setBody] = useState('');
  const post = useMutation({
    mutationFn: () => addNote(submissionId, body.trim()),
    onSuccess: () => {
      setBody('');
      void qc.invalidateQueries({ queryKey: ['notes', submissionId] });
    },
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle>Staff notes</CardTitle>
        <p className="text-sm text-muted-foreground">Only staff see these.</p>
      </CardHeader>
      <CardContent className="space-y-4">
        {q.isLoading ? (
          <LoadingRow />
        ) : (q.data?.items ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">No notes yet.</p>
        ) : (
          <ul className="space-y-3">
            {q.data!.items.map((n) => (
              <li key={n.id} className="text-sm">
                <div className="text-xs text-muted-foreground">
                  <span className="font-semibold text-foreground">{n.author_name}</span> · {formatWhen(n.created_at)}
                </div>
                <p className="mt-0.5 whitespace-pre-wrap">{n.body}</p>
              </li>
            ))}
          </ul>
        )}
        {canWrite ? (
          <form
            className="space-y-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (body.trim()) post.mutate();
            }}
          >
            <textarea
              className={cn(textareaClasses, 'min-h-16')}
              placeholder="Add a note for the team"
              value={body}
              onChange={(e) => setBody(e.target.value)}
            />
            <Button size="sm" type="submit" disabled={!body.trim() || post.isPending}>
              Post note
            </Button>
            {post.error ? <p className="text-sm text-destructive">{errorText(post.error)}</p> : null}
          </form>
        ) : null}
      </CardContent>
    </Card>
  );
}
