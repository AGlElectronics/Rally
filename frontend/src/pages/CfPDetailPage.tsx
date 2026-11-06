import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router';
import { ArrowLeft } from 'lucide-react';
import { getCfP, listEventTypes } from '@/api/cfp';
import { errorText } from '@/api/client';
import { CfPSettings } from '@/components/cfp/CfPSettings';
import { FormBuilder } from '@/components/cfp/FormBuilder';
import { SubmissionsTab } from '@/components/cfp/SubmissionsTab';
import { PageContainer, PageHeader } from '@/components/ui/PageHeader';
import { Pill } from '@/components/ui/Pill';
import { ErrorRow, LoadingRow } from '@/components/ui/States';
import { PERMS, useMe } from '@/hooks/useMe';
import { cn } from '@/lib/utils';

export type CfPTab = 'submissions' | 'form' | 'settings';

const TABS: { id: CfPTab; label: string }[] = [
  { id: 'submissions', label: 'Submissions' },
  { id: 'form', label: 'Form' },
  { id: 'settings', label: 'Settings' },
];

export function CfPDetailPage() {
  const { cfpId } = useParams({ from: '/app/cfp/$cfpId' });
  const { tab } = useSearch({ from: '/app/cfp/$cfpId' });
  const navigate = useNavigate();
  const { can } = useMe();
  const cfp = useQuery({ queryKey: ['cfp', cfpId], queryFn: () => getCfP(cfpId) });
  const types = useQuery({
    queryKey: ['event-types', cfp.data?.edition_id],
    queryFn: () => listEventTypes(cfp.data!.edition_id),
    enabled: !!cfp.data,
  });
  const canEdit = can(PERMS.cfpManage);

  if (cfp.isLoading) return <LoadingRow />;
  if (cfp.error || !cfp.data) return <ErrorRow message={errorText(cfp.error) || 'Call not found.'} />;
  const c = cfp.data;
  const eventTypes = types.data?.items ?? [];

  return (
    <PageContainer>
      <Link to="/cfp" className="mb-3 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft className="h-4 w-4" />
        All calls
      </Link>
      <PageHeader
        title={c.title}
        description={c.status === 'open' ? 'Open for submissions.' : c.status === 'draft' ? 'Draft: not visible to submitters yet.' : 'Closed.'}
        actions={<Pill tone={c.status === 'open' ? 'success' : c.status === 'closed' ? 'warning' : 'neutral'}>{c.status}</Pill>}
      />
      <div className="mb-5 flex gap-1 border-b border-border">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            onClick={() => void navigate({ to: '/cfp/$cfpId', params: { cfpId }, search: { tab: t.id }, replace: true })}
            className={cn(
              '-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors',
              tab === t.id ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground',
            )}
          >
            {t.label}
          </button>
        ))}
      </div>
      {tab === 'form' ? (
        <FormBuilder cfp={c} eventTypes={eventTypes} canEdit={canEdit} />
      ) : tab === 'settings' ? (
        <CfPSettings cfp={c} eventTypes={eventTypes} canEdit={canEdit} />
      ) : (
        <SubmissionsTab cfp={c} eventTypes={eventTypes} />
      )}
    </PageContainer>
  );
}
