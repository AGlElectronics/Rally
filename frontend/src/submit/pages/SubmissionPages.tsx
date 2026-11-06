import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useNavigate, useParams } from '@tanstack/react-router';
import { ArrowLeft, FileText, LogOut, Undo2 } from 'lucide-react';
import { errorText } from '@/api/client';
import { AnswersView } from '@/components/forms/AnswersView';
import { Button, LinkButton } from '@/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { Pill, StatePill } from '@/components/ui/Pill';
import { EmptyState, ErrorRow, LoadingRow } from '@/components/ui/States';
import { openFile } from '@/lib/upload';
import { PUBLIC_API, getCall, getMine, leaveSubmission, listMine, withdrawMine, type PublicCall } from '@/submit/api';
import { PeoplePanel } from '@/submit/PeoplePanel';
import { SubmissionEditor } from '@/submit/SubmissionEditor';
import { useProfile } from '@/submit/SubmitLayout';
import { CallWindow, formatDate } from '@/submit/pages/CallsPage';

function CallIntro({ call }: { call: PublicCall }) {
  return (
    <div className="mb-6">
      <Link to="/" className="mb-3 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft className="h-4 w-4" />
        Open calls
      </Link>
      <p className="text-xs font-semibold uppercase tracking-wide text-primary">{call.edition_title}</p>
      <h1 className="font-heading text-2xl font-extrabold tracking-tight">{call.title}</h1>
      <p className="mt-0.5 text-sm text-muted-foreground">
        <CallWindow call={call} />
      </p>
      {call.description ? <p className="mt-3 max-w-3xl whitespace-pre-line text-sm">{call.description}</p> : null}
    </div>
  );
}

export function NewSubmissionPage() {
  const { cfpId } = useParams({ from: '/site/calls/$cfpId/new' });
  const call = useQuery({ queryKey: ['call', cfpId], queryFn: () => getCall(cfpId) });
  if (call.isLoading) return <LoadingRow />;
  if (call.error || !call.data) return <ErrorRow message={errorText(call.error) || 'This call does not exist.'} />;
  return (
    <div>
      <CallIntro call={call.data} />
      {call.data.accepting_now ? (
        <SubmissionEditor call={call.data} />
      ) : (
        <EmptyState title="This call isn't taking submissions right now" />
      )}
    </div>
  );
}

export function SubmissionPage() {
  const { submissionId } = useParams({ from: '/site/submissions/$submissionId' });
  const qc = useQueryClient();
  const navigate = useNavigate();
  const sub = useQuery({ queryKey: ['my-submission', submissionId], queryFn: () => getMine(submissionId) });
  const call = useQuery({
    queryKey: ['call', sub.data?.cfp_id],
    queryFn: () => getCall(sub.data!.cfp_id),
    enabled: !!sub.data,
  });
  const profile = useProfile(true);
  const withdraw = useMutation({
    mutationFn: () => withdrawMine(submissionId),
    onSuccess: (s) => {
      qc.setQueryData(['my-submission', s.id], s);
      void qc.invalidateQueries({ queryKey: ['my-submissions'] });
    },
  });
  const leave = useMutation({
    mutationFn: () => leaveSubmission(submissionId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['my-submissions'] });
      void navigate({ to: '/submissions' });
    },
  });

  if (sub.isLoading || call.isLoading || profile.isLoading) return <LoadingRow />;
  if (sub.error || !sub.data) return <ErrorRow message={errorText(sub.error) || 'Submission not found.'} />;
  if (call.error || !call.data) return <ErrorRow message={errorText(call.error) || 'Call not found.'} />;
  const s = sub.data;
  const isOwner = profile.data?.id === s.submitter_user_id;
  const editable = isOwner && s.cfp_accepting && (s.state === 'draft' || s.state === 'submitted');
  const decided = s.state === 'accepted' || s.state === 'rejected';

  return (
    <div>
      <CallIntro call={call.data} />
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <StatePill state={s.state} labels={s.status_labels} />
        {!isOwner ? <Pill>you're a co-presenter</Pill> : null}
        {s.submitted_at ? <span className="text-sm text-muted-foreground">Submitted {formatDate(s.submitted_at)}</span> : null}
        <div className="ml-auto flex gap-2">
          {isOwner && (s.state === 'submitted' || s.state === 'draft') ? (
            <Button
              size="sm"
              variant="ghost"
              disabled={withdraw.isPending}
              onClick={() => {
                if (confirm('Withdraw this submission? You can not undo this.')) withdraw.mutate();
              }}
            >
              <Undo2 className="h-4 w-4" />
              Withdraw
            </Button>
          ) : null}
          {!isOwner ? (
            <Button
              size="sm"
              variant="ghost"
              disabled={leave.isPending}
              onClick={() => {
                if (confirm('Remove yourself from this submission?')) leave.mutate();
              }}
            >
              <LogOut className="h-4 w-4" />
              Leave
            </Button>
          ) : null}
        </div>
      </div>
      {withdraw.error || leave.error ? (
        <p className="mb-3 text-sm text-destructive">{errorText(withdraw.error || leave.error)}</p>
      ) : null}

      {decided && s.decision_note ? (
        <Card className="mb-4 border-primary/40">
          <CardHeader>
            <CardTitle>Message from the organisers</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="whitespace-pre-wrap text-sm">{s.decision_note}</p>
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,22rem)]">
        <div>
          {editable ? (
            <SubmissionEditor key={s.id} call={call.data} submission={s} />
          ) : (
            <Card>
              <CardHeader>
                <CardTitle>{s.title}</CardTitle>
                <p className="text-sm text-muted-foreground">
                  {[s.event_type_name, s.duration_minutes ? `${s.duration_minutes} min` : ''].filter(Boolean).join(' · ')}
                </p>
              </CardHeader>
              <CardContent>
                <AnswersView
                  fields={call.data.form.fields}
                  answers={s.answers ?? {}}
                  files={s.files}
                  onOpenFile={(id) => void openFile(PUBLIC_API, id).catch((e) => alert(errorText(e)))}
                />
              </CardContent>
            </Card>
          )}
        </div>
        <div>
          <PeoplePanel submission={s} canEdit={editable} />
        </div>
      </div>
    </div>
  );
}

export function MySubmissionsPage() {
  const q = useQuery({ queryKey: ['my-submissions'], queryFn: listMine });
  const profile = useProfile(true);
  return (
    <div>
      <h1 className="mb-6 font-heading text-2xl font-extrabold tracking-tight">My submissions</h1>
      {q.isLoading ? (
        <LoadingRow />
      ) : q.error ? (
        <ErrorRow message={errorText(q.error)} />
      ) : (q.data?.items ?? []).length === 0 ? (
        <EmptyState
          icon={FileText}
          title="Nothing yet"
          description="Ideas you submit, or are invited to present, show up here."
          action={<LinkButton to="/">See open calls</LinkButton>}
        />
      ) : (
        <Card className="overflow-hidden">
          <ul className="divide-y divide-border">
            {q.data!.items.map((s) => (
              <li key={s.id}>
                <Link
                  to="/submissions/$submissionId"
                  params={{ submissionId: s.id }}
                  className="flex items-center gap-3 px-4 py-3 transition-colors hover:bg-secondary"
                >
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium">{s.title || 'Untitled'}</div>
                    <div className="truncate text-xs text-muted-foreground">
                      {s.edition_title} · {s.cfp_title}
                      {profile.data && profile.data.id !== s.submitter_user_id ? ' · co-presenter' : ''}
                    </div>
                  </div>
                  <StatePill state={s.state} labels={s.status_labels} />
                </Link>
              </li>
            ))}
          </ul>
        </Card>
      )}
    </div>
  );
}
