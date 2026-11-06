import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Check, Copy, RefreshCw, UserPlus, X } from 'lucide-react';
import { errorText } from '@/api/client';
import { Button } from '@/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { Pill } from '@/components/ui/Pill';
import { addPerson, reissueInvite, removePerson, type Submission } from '@/submit/api';

const CODE = /^[A-Za-z0-9]{4}-?[A-Za-z0-9]{4}$/;

export function inviteLink(code: string): string {
  return `${window.location.origin}/invite/${code}`;
}

export function CopyButton({ text, label = 'Copy' }: { text: string; label?: string }) {
  const [done, setDone] = useState(false);
  return (
    <Button
      type="button"
      size="sm"
      variant="secondary"
      onClick={() =>
        void navigator.clipboard.writeText(text).then(() => {
          setDone(true);
          setTimeout(() => setDone(false), 1500);
        })
      }
    >
      {done ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
      {done ? 'Copied' : label}
    </Button>
  );
}

/** Co-presenters of a submission; only the submitter can change the list. */
export function PeoplePanel({ submission, canEdit }: { submission: Submission; canEdit: boolean }) {
  const qc = useQueryClient();
  const [who, setWho] = useState('');
  const done = (s: Submission) => {
    qc.setQueryData(['my-submission', s.id], s);
    void qc.invalidateQueries({ queryKey: ['my-submissions'] });
  };
  const add = useMutation({
    mutationFn: () => {
      const v = who.trim();
      return addPerson(submission.id, CODE.test(v) ? { presenter_code: v } : { email: v });
    },
    onSuccess: (s) => {
      setWho('');
      done(s);
    },
  });
  const remove = useMutation({ mutationFn: (personId: string) => removePerson(submission.id, personId), onSuccess: done });
  const reissue = useMutation({ mutationFn: (personId: string) => reissueInvite(submission.id, personId), onSuccess: done });
  const error = add.error || remove.error || reissue.error;

  return (
    <Card>
      <CardHeader>
        <CardTitle>People</CardTitle>
        {canEdit ? (
          <p className="text-sm text-muted-foreground">
            Everyone presenting needs an account here. Add them by their presenter code (on their profile page) or by
            email. If the email doesn't belong to a confirmed account yet, you get an invite link to send them.
          </p>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-4">
        <ul className="space-y-2">
          {submission.people.map((p) => (
            <li key={p.id} className="rounded-md border border-border px-3 py-2 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{p.display_name || p.invite_email || 'Invited presenter'}</span>
                {p.role === 'owner' ? <Pill tone="primary">submitter</Pill> : null}
                {p.status === 'invited' ? <Pill tone="warning">invite pending</Pill> : null}
                {canEdit && p.role !== 'owner' ? (
                  <div className="ml-auto flex items-center gap-1">
                    {p.status === 'invited' ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        title="New link (the old one stops working)"
                        disabled={reissue.isPending}
                        onClick={() => reissue.mutate(p.id)}
                      >
                        <RefreshCw className="h-3.5 w-3.5" />
                        New link
                      </Button>
                    ) : null}
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      aria-label="Remove"
                      disabled={remove.isPending}
                      onClick={() => {
                        if (confirm('Remove this person from the submission?')) remove.mutate(p.id);
                      }}
                    >
                      <X className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                ) : null}
              </div>
              {p.status === 'invited' && p.invite_code ? (
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  <code className="min-w-0 flex-1 truncate rounded bg-secondary px-2 py-1 text-xs">{inviteLink(p.invite_code)}</code>
                  <CopyButton text={inviteLink(p.invite_code)} label="Copy link" />
                </div>
              ) : null}
            </li>
          ))}
        </ul>
        {canEdit ? (
          <form
            className="flex flex-wrap gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (who.trim()) add.mutate();
            }}
          >
            <Input
              className="min-w-56 flex-1"
              value={who}
              placeholder="Presenter code (ABCD-1234) or email"
              onChange={(e) => setWho(e.target.value)}
            />
            <Button type="submit" variant="secondary" disabled={!who.trim() || add.isPending}>
              <UserPlus className="h-4 w-4" />
              Add
            </Button>
          </form>
        ) : null}
        {error ? <p className="text-sm text-destructive">{errorText(error)}</p> : null}
      </CardContent>
    </Card>
  );
}
