import { useQuery } from '@tanstack/react-query';
import { CalendarRange, Users2 } from 'lucide-react';
import { LinkButton } from '@/components/ui/Button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/Card';
import { PageContainer, PageHeader } from '@/components/ui/PageHeader';
import { EmptyState, ErrorRow, LoadingRow } from '@/components/ui/States';
import { listOrganizations } from '@/api/organizations';
import { useMe } from '@/hooks/useMe';

export function DashboardPage() {
  const { me } = useMe();
  const orgsQ = useQuery({
    queryKey: ['organizations'],
    queryFn: ({ signal }) => listOrganizations(signal),
  });

  return (
    <PageContainer>
      <PageHeader
        title={`Hello${me?.display_name ? `, ${me.display_name}` : ''}`}
        description="A home base for your conventions, editions, and programmes."
      />

      <div className="grid gap-4 sm:grid-cols-2">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <span className="rounded-pill bg-primary-soft p-1.5">
                <Users2 className="w-4 h-4 text-primary" />
              </span>
              <CardTitle>Organizations</CardTitle>
            </div>
            <CardDescription>
              Teams you belong to. Role is applied per organization; some actions
              are also edition-scoped.
            </CardDescription>
          </CardHeader>
          <CardContent>
            {orgsQ.isLoading ? (
              <LoadingRow />
            ) : orgsQ.isError ? (
              <ErrorRow message="Could not load organizations." />
            ) : orgsQ.data && orgsQ.data.items.length > 0 ? (
              <>
                <p className="text-sm text-muted-foreground">
                  You belong to{' '}
                  <span className="font-semibold text-foreground">{orgsQ.data.total}</span>
                  {' '}organization{orgsQ.data.total === 1 ? '' : 's'}.
                </p>
                <div className="mt-4">
                  <LinkButton to="/organizations" variant="secondary">
                    View organizations
                  </LinkButton>
                </div>
              </>
            ) : (
              <EmptyState
                icon={Users2}
                title="No organizations yet"
                description="Ask an admin to add you to an organisation, or seed one via the migrations."
              />
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <span className="rounded-pill bg-primary-soft p-1.5">
                <CalendarRange className="w-4 h-4 text-primary" />
              </span>
              <CardTitle>Programme (soon)</CardTitle>
            </div>
            <CardDescription>
              Call for proposals, review, scheduling, and publishing arrive with
              the first vertical slice.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <EmptyState
              icon={CalendarRange}
              title="Not yet implemented"
              description="The scaffold ships Dashboard + Organizations today; the programme pages ship with the first vertical slice."
            />
          </CardContent>
        </Card>
      </div>
    </PageContainer>
  );
}
