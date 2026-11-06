import { useQuery } from '@tanstack/react-query';
import { useNavigate, useSearch } from '@tanstack/react-router';
import { CalendarClock } from 'lucide-react';
import { errorText } from '@/api/client';
import { getSchedule } from '@/api/schedule';
import { EditionPicker } from '@/components/EditionPicker';
import { LiveTab } from '@/components/schedule/LiveTab';
import { ScheduleBuilder } from '@/components/schedule/ScheduleBuilder';
import { SetupTab } from '@/components/schedule/SetupTab';
import { VersionsTab } from '@/components/schedule/VersionsTab';
import { PageContainer, PageHeader } from '@/components/ui/PageHeader';
import { Pill } from '@/components/ui/Pill';
import { EmptyState, ErrorRow, LoadingRow } from '@/components/ui/States';
import { useEdition } from '@/hooks/useEdition';
import { cn } from '@/lib/utils';

export type ScheduleTab = 'build' | 'live' | 'versions' | 'setup';

const TABS: { id: ScheduleTab; label: string }[] = [
  { id: 'build', label: 'Build' },
  { id: 'live', label: 'Live day' },
  { id: 'versions', label: 'Versions' },
  { id: 'setup', label: 'Setup' },
];

export function SchedulePage() {
  const { tab } = useSearch({ from: '/app/schedule' });
  const navigate = useNavigate();
  const { editions, edition, setEditionId, isLoading, error } = useEdition();
  const schedule = useQuery({
    queryKey: ['schedule', edition?.id],
    queryFn: () => getSchedule(edition!.id),
    enabled: !!edition,
  });
  const sc = schedule.data;
  const go = (t: ScheduleTab) => void navigate({ to: '/schedule', search: { tab: t }, replace: true });

  return (
    <PageContainer className="max-w-none">
      <PageHeader
        title="Schedule"
        description="Drag accepted submissions onto rooms and times, publish, and handle changes on the day."
        actions={
          <>
            {sc?.published ? (
              <Pill tone={sc.draft_changed ? 'warning' : 'success'}>
                Published v{sc.published.version}
                {sc.draft_changed ? ' · draft has changes' : ''}
              </Pill>
            ) : sc ? (
              <Pill>Not published yet</Pill>
            ) : null}
            <EditionPicker editions={editions} edition={edition} onChange={setEditionId} />
          </>
        }
      />
      <div className="mb-4 flex gap-1 border-b border-border">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            onClick={() => go(t.id)}
            className={cn(
              '-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors',
              tab === t.id ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground',
            )}
          >
            {t.label}
          </button>
        ))}
      </div>
      {isLoading || schedule.isLoading ? (
        <LoadingRow />
      ) : error || schedule.error ? (
        <ErrorRow message={errorText(error || schedule.error)} />
      ) : !edition || !sc ? (
        <EmptyState icon={CalendarClock} title="No edition" description="Create an edition first." />
      ) : tab === 'setup' ? (
        <SetupTab schedule={sc} />
      ) : tab === 'versions' ? (
        <VersionsTab schedule={sc} />
      ) : tab === 'live' ? (
        <LiveTab schedule={sc} onSetup={() => go('setup')} />
      ) : (
        <ScheduleBuilder schedule={sc} onSetup={() => go('setup')} />
      )}
    </PageContainer>
  );
}
