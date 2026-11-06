import { useRef, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { Save, Send } from 'lucide-react';
import { errorText, issuesOf } from '@/api/client';
import { FormFields } from '@/components/forms/FormFields';
import { Button } from '@/components/ui/Button';
import { Card, CardContent } from '@/components/ui/Card';
import { Input, Label } from '@/components/ui/Input';
import { selectClasses } from '@/components/ui/Pill';
import type { Answers } from '@/lib/forms';
import { openFile, uploadFile } from '@/lib/upload';
import { cn } from '@/lib/utils';
import {
  PUBLIC_API,
  createMine,
  submitMine,
  updateMine,
  type PublicCall,
  type Submission,
  type SubmissionBody,
} from '@/submit/api';

/**
 * The submission form: built-in fields (title, event type, length) plus the
 * call's own questions. Drafts can be saved incomplete; submitting (and saving
 * an already submitted entry) requires every required answer.
 */
export function SubmissionEditor({ call, submission }: { call: PublicCall; submission?: Submission }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [title, setTitle] = useState(submission?.title ?? '');
  const [eventTypeId, setEventTypeId] = useState(submission?.event_type_id ?? '');
  const [duration, setDuration] = useState<number | undefined>(submission?.duration_minutes ?? undefined);
  const [answers, setAnswers] = useState<Answers>((submission?.answers as Answers) ?? {});
  const [saved, setSaved] = useState(false);
  const isSubmitted = submission?.state === 'submitted';
  const type = call.event_types.find((t) => t.id === eventTypeId);

  const body = (): SubmissionBody => ({
    title,
    answers,
    event_type_id: eventTypeId || null,
    // On updates 0 clears the requested length; null would mean "unchanged".
    duration_minutes: duration ?? (submission ? 0 : null),
  });

  // Set once a new submission exists, so a failed submit step followed by a
  // retry updates that draft instead of creating a second one.
  const createdId = useRef<string | undefined>(undefined);
  const save = useMutation({
    mutationFn: async (andSubmit: boolean) => {
      const id = submission?.id ?? createdId.current;
      let s = id ? await updateMine(id, body()) : await createMine(call.id, body());
      createdId.current = s.id;
      if (andSubmit) s = await submitMine(s.id);
      return s;
    },
    onSuccess: (s) => {
      qc.setQueryData(['my-submission', s.id], s);
      void qc.invalidateQueries({ queryKey: ['my-submissions'] });
      setSaved(true);
      if (!submission) void navigate({ to: '/submissions/$submissionId', params: { submissionId: s.id }, replace: true });
    },
    onMutate: () => setSaved(false),
  });
  const issues = issuesOf(save.error);
  const err = (k: string) => (issues[k] ? <p className="mt-1 text-xs text-destructive">{issues[k]}</p> : null);

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate(!isSubmitted);
      }}
    >
      <Card>
        <CardContent className="space-y-5">
          <div>
            <Label htmlFor="title">
              Title <span className="text-destructive">*</span>
            </Label>
            <Input id="title" value={title} maxLength={200} placeholder="What is it called?" onChange={(e) => setTitle(e.target.value)} />
            {err('title')}
          </div>

          {call.event_types.length > 0 ? (
            <div className="grid gap-4 sm:grid-cols-[1fr_12rem]">
              <div>
                <Label htmlFor="type">
                  Kind of event <span className="text-destructive">*</span>
                </Label>
                <select
                  id="type"
                  className={cn(selectClasses, 'w-full')}
                  value={eventTypeId}
                  onChange={(e) => setEventTypeId(e.target.value)}
                >
                  <option value="">Choose…</option>
                  {call.event_types.map((t) => (
                    <option key={t.id} value={t.id}>
                      {t.name}
                    </option>
                  ))}
                </select>
                {err('event_type_id')}
              </div>
              <div>
                <Label htmlFor="duration">Length (minutes)</Label>
                <Input
                  id="duration"
                  type="number"
                  min={5}
                  max={1440}
                  step={5}
                  value={duration ?? ''}
                  placeholder={type ? String(type.default_minutes) : 'optional'}
                  onChange={(e) => setDuration(e.target.value === '' ? undefined : Number(e.target.value))}
                />
                {err('duration_minutes')}
              </div>
            </div>
          ) : null}

          <FormFields
            fields={call.form.fields}
            answers={answers}
            onChange={(id, v) => setAnswers((a) => ({ ...a, [id]: v }))}
            errors={issues}
            files={submission?.files}
            onUpload={(fieldId, file) => uploadFile(PUBLIC_API, call.id, fieldId, file)}
            onOpenFile={(id) => void openFile(PUBLIC_API, id).catch((e) => alert(errorText(e)))}
          />
        </CardContent>
      </Card>

      <div className="sticky bottom-3 z-10 mt-4 flex flex-wrap items-center gap-3 rounded-card border border-border bg-card p-3 shadow-pop">
        {isSubmitted ? (
          <Button type="submit" disabled={save.isPending}>
            <Save className="h-4 w-4" />
            Save changes
          </Button>
        ) : (
          <>
            <Button type="submit" disabled={save.isPending}>
              <Send className="h-4 w-4" />
              Submit
            </Button>
            <Button type="button" variant="secondary" disabled={save.isPending} onClick={() => save.mutate(false)}>
              <Save className="h-4 w-4" />
              Save draft
            </Button>
          </>
        )}
        {save.isPending ? <span className="text-sm text-muted-foreground">Saving…</span> : null}
        {saved && !save.isPending ? <span className="text-sm text-success">Saved.</span> : null}
        {save.error ? (
          <span className="text-sm text-destructive">
            {Object.keys(issues).length ? 'Please check the highlighted answers.' : errorText(save.error)}
          </span>
        ) : null}
      </div>
    </form>
  );
}
