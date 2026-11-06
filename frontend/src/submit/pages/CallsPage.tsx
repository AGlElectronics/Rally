import { useQuery } from '@tanstack/react-query';
import { CalendarClock, Megaphone } from 'lucide-react';
import { errorText } from '@/api/client';
import { Link } from '@tanstack/react-router';
import { buttonClasses } from '@/components/ui/Button';
import { Card, CardContent } from '@/components/ui/Card';
import { ColorDot } from '@/components/ui/Pill';
import { EmptyState, ErrorRow, LoadingRow } from '@/components/ui/States';
import { listCalls, type PublicCall } from '@/submit/api';

export function formatDate(iso?: string | null): string {
  return iso ? new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : '';
}

export function CallWindow({ call }: { call: PublicCall }) {
  const now = Date.now();
  if (call.opens_at && new Date(call.opens_at).getTime() > now) {
    return <>Opens {formatDate(call.opens_at)}</>;
  }
  if (!call.accepting_now) return <>Closed</>;
  return <>{call.closes_at ? `Open until ${formatDate(call.closes_at)}` : 'Open'}</>;
}

export function CallsPage() {
  const calls = useQuery({ queryKey: ['calls'], queryFn: listCalls });

  return (
    <div>
      <h1 className="font-heading text-3xl font-extrabold tracking-tight">Bring your idea to the programme</h1>
      <p className="mt-2 max-w-2xl text-muted-foreground">
        Panels, workshops, performances, meetups: pick an open call below and tell us what you have in mind. You can save a
        draft and come back to it until the call closes.
      </p>

      <div className="mt-8">
        {calls.isLoading ? (
          <LoadingRow />
        ) : calls.error ? (
          <ErrorRow message={errorText(calls.error)} />
        ) : (calls.data?.items ?? []).length === 0 ? (
          <EmptyState icon={Megaphone} title="No open calls right now" description="Check back later; calls open ahead of each event." />
        ) : (
          <div className="grid gap-4">
            {calls.data!.items.map((c) => (
              <Card key={c.id}>
                <CardContent className="space-y-3">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <p className="text-xs font-semibold uppercase tracking-wide text-primary">{c.edition_title}</p>
                      <h2 className="font-heading text-xl font-bold">{c.title}</h2>
                      <p className="mt-0.5 flex items-center gap-1.5 text-sm text-muted-foreground">
                        <CalendarClock className="h-4 w-4" />
                        <CallWindow call={c} />
                      </p>
                    </div>
                    {c.accepting_now ? (
                      <Link to="/calls/$cfpId/new" params={{ cfpId: c.id }} className={buttonClasses()}>
                        Submit an idea
                      </Link>
                    ) : null}
                  </div>
                  {c.description ? <p className="whitespace-pre-line text-sm">{c.description}</p> : null}
                  {c.event_types.length > 0 ? (
                    <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                      {c.event_types.map((t) => (
                        <span key={t.id} className="flex items-center gap-1.5">
                          <ColorDot color={t.color} />
                          {t.name}
                        </span>
                      ))}
                    </div>
                  ) : null}
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
