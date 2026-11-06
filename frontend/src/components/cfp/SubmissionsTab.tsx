import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { Inbox, Search, Star } from 'lucide-react';
import { listSubmissions, type CallForPapers, type EventType } from '@/api/cfp';
import { errorText } from '@/api/client';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { ColorDot, StatePill, selectClasses } from '@/components/ui/Pill';
import { EmptyState, ErrorRow, LoadingRow } from '@/components/ui/States';
import { SUBMISSION_STATES } from '@/lib/forms';

type Sort = 'newest' | 'oldest' | 'score' | 'title';

export function SubmissionsTab({ cfp, eventTypes }: { cfp: CallForPapers; eventTypes: EventType[] }) {
  const [state, setState] = useState<string>('submitted');
  const [typeId, setTypeId] = useState('');
  const [query, setQuery] = useState('');
  const [sort, setSort] = useState<Sort>('newest');
  const subs = useQuery({
    queryKey: ['submissions', cfp.edition_id, cfp.id],
    queryFn: () => listSubmissions(cfp.edition_id, { cfp_id: cfp.id }),
  });
  const scored = cfp.review_mode === 'scored';
  const color = (id?: string | null) => eventTypes.find((t) => t.id === id)?.color ?? '#94a3b8';

  const all = subs.data?.items ?? [];
  const counts = useMemo(() => {
    const c: Record<string, number> = {};
    for (const s of all) c[s.state] = (c[s.state] ?? 0) + 1;
    return c;
  }, [all]);

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = all.filter(
      (s) =>
        (!state || s.state === state) &&
        (!typeId || s.event_type_id === typeId) &&
        (!q || s.title.toLowerCase().includes(q) || s.submitter_name.toLowerCase().includes(q)),
    );
    const time = (s: (typeof list)[number]) => new Date(s.submitted_at ?? s.created_at).getTime();
    return list.sort((a, b) => {
      switch (sort) {
        case 'oldest':
          return time(a) - time(b);
        case 'score':
          return (b.score_avg ?? -1) - (a.score_avg ?? -1);
        case 'title':
          return a.title.localeCompare(b.title);
        default:
          return time(b) - time(a);
      }
    });
  }, [all, state, typeId, query, sort]);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-56 flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input className="pl-9" placeholder="Search title or submitter" value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>
        <select aria-label="State" className={selectClasses} value={state} onChange={(e) => setState(e.target.value)}>
          <option value="">All states ({all.length})</option>
          {SUBMISSION_STATES.map((s) => (
            <option key={s} value={s}>
              {s} ({counts[s] ?? 0})
            </option>
          ))}
        </select>
        {eventTypes.length > 0 ? (
          <select aria-label="Event type" className={selectClasses} value={typeId} onChange={(e) => setTypeId(e.target.value)}>
            <option value="">All types</option>
            {eventTypes.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
        ) : null}
        <select aria-label="Sort" className={selectClasses} value={sort} onChange={(e) => setSort(e.target.value as Sort)}>
          <option value="newest">Newest first</option>
          <option value="oldest">Oldest first</option>
          {scored ? <option value="score">Highest score</option> : null}
          <option value="title">Title</option>
        </select>
      </div>

      {subs.isLoading ? (
        <LoadingRow />
      ) : subs.error ? (
        <ErrorRow message={errorText(subs.error)} />
      ) : rows.length === 0 ? (
        <EmptyState icon={Inbox} title="Nothing here" description={all.length ? 'No submissions match these filters.' : 'No submissions yet.'} />
      ) : (
        <Card className="overflow-hidden">
          <ul className="divide-y divide-border">
            {rows.map((s) => (
              <li key={s.id}>
                <Link
                  to="/submissions/$submissionId"
                  params={{ submissionId: s.id }}
                  className="flex items-center gap-3 px-4 py-3 transition-colors hover:bg-secondary"
                >
                  <ColorDot color={color(s.event_type_id)} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium">{s.title}</div>
                    <div className="truncate text-xs text-muted-foreground">
                      {s.submitter_name}
                      {s.event_type_name ? ` · ${s.event_type_name}` : ''}
                      {s.duration_minutes ? ` · ${s.duration_minutes} min` : ''}
                    </div>
                  </div>
                  {scored ? (
                    <span className="flex items-center gap-1 text-sm tabular-nums text-muted-foreground" title="Average score">
                      <Star className="h-3.5 w-3.5" />
                      {s.score_avg != null ? s.score_avg.toFixed(1) : '–'}
                      <span className="text-xs">({s.score_count ?? 0})</span>
                    </span>
                  ) : null}
                  <StatePill state={s.state} />
                </Link>
              </li>
            ))}
          </ul>
        </Card>
      )}
    </div>
  );
}
