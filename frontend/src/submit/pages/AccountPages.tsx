import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useNavigate, useParams } from '@tanstack/react-router';
import { Check, MailWarning, UserPlus } from 'lucide-react';
import { errorText } from '@/api/client';
import { Button, buttonClasses } from '@/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { Pill } from '@/components/ui/Pill';
import { ErrorRow, LoadingRow } from '@/components/ui/States';
import { acceptInvite, declineInvite, previewInvite } from '@/submit/api';
import { CopyButton } from '@/submit/PeoplePanel';
import { useProfile } from '@/submit/SubmitLayout';

export function ProfilePage() {
  const profile = useProfile(true);
  if (profile.isLoading) return <LoadingRow />;
  if (profile.error || !profile.data) return <ErrorRow message={errorText(profile.error) || 'Could not load your profile.'} />;
  const p = profile.data;
  return (
    <div className="max-w-xl space-y-6">
      <h1 className="font-heading text-2xl font-extrabold tracking-tight">Your profile</h1>
      <Card>
        <CardContent className="space-y-1 text-sm">
          <p className="font-heading text-lg font-bold">{p.display_name}</p>
          <p className="flex items-center gap-2 text-muted-foreground">
            {p.email}
            {p.email_verified ? <Pill tone="success">confirmed</Pill> : <Pill tone="warning">not confirmed</Pill>}
          </p>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Your presenter code</CardTitle>
          <p className="text-sm text-muted-foreground">
            Give this code to whoever submits an idea you'll present together. With it they add you directly, no invite
            link needed.
          </p>
        </CardHeader>
        <CardContent className="flex flex-wrap items-center gap-3">
          <code className="rounded-md bg-secondary px-4 py-2 font-mono text-2xl font-bold tracking-widest">{p.presenter_code}</code>
          <CopyButton text={p.presenter_code} />
        </CardContent>
      </Card>
      {!p.email_verified ? (
        <p className="flex items-start gap-2 text-sm text-muted-foreground">
          <MailWarning className="mt-0.5 h-4 w-4 shrink-0" />
          Confirm your email address so people can add you by email too.
        </p>
      ) : null}
    </div>
  );
}

export function InvitePage() {
  const { code } = useParams({ from: '/site/invite/$code' });
  const qc = useQueryClient();
  const navigate = useNavigate();
  const invite = useQuery({ queryKey: ['invite', code], queryFn: () => previewInvite(code), retry: false });
  const accept = useMutation({
    mutationFn: () => acceptInvite(code),
    onSuccess: (s) => {
      void qc.invalidateQueries({ queryKey: ['my-submissions'] });
      void navigate({ to: '/submissions/$submissionId', params: { submissionId: s.id } });
    },
  });
  const decline = useMutation({ mutationFn: () => declineInvite(code), onSuccess: () => void navigate({ to: '/' }) });

  if (invite.isLoading) return <LoadingRow />;
  if (invite.error || !invite.data) {
    return <ErrorRow message="This invite link is not valid any more. Ask the submitter for a new one." />;
  }
  const i = invite.data;
  return (
    <Card className="mx-auto max-w-lg">
      <CardHeader>
        <p className="text-xs font-semibold uppercase tracking-wide text-primary">
          {i.edition_title} · {i.cfp_title}
        </p>
        <CardTitle className="text-xl">{i.title}</CardTitle>
        <p className="text-sm text-muted-foreground">{i.invited_by} invited you to present this together.</p>
      </CardHeader>
      <CardContent className="space-y-3">
        {i.already_on_it ? (
          <Link to="/submissions/$submissionId" params={{ submissionId: i.submission_id }} className={buttonClasses()}>
            <Check className="h-4 w-4" />
            You're already on it: open it
          </Link>
        ) : (
          <div className="flex flex-wrap gap-2">
            <Button onClick={() => accept.mutate()} disabled={accept.isPending || decline.isPending}>
              <UserPlus className="h-4 w-4" />
              Join as co-presenter
            </Button>
            <Button variant="ghost" onClick={() => decline.mutate()} disabled={accept.isPending || decline.isPending}>
              Decline
            </Button>
          </div>
        )}
        {!i.submission_open ? (
          <p className="text-sm text-muted-foreground">The call has closed, so the submission itself can't be changed any more.</p>
        ) : null}
        {accept.error || decline.error ? <p className="text-sm text-destructive">{errorText(accept.error || decline.error)}</p> : null}
      </CardContent>
    </Card>
  );
}
