import { useQuery } from '@tanstack/react-query';
import { CalendarClock, Inbox, Shapes, Users2 } from 'lucide-react';
import { LinkButton } from '@/components/ui/Button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/Card';
import { PageContainer, PageHeader } from '@/components/ui/PageHeader';
import { EmptyState, ErrorRow, LoadingRow } from '@/components/ui/States';
import { listOrganizations } from '@/api/organizations';
import { useMe, PERMS } from '@/hooks/useMe';

export function DashboardPage() {
  const { me, canAny } = useMe();
  const orgsQ = useQuery({
    queryKey: ['organizations'],
    queryFn: ({ signal }) => listOrganizations(signal),
  });

  const canProgramme = canAny(
    PERMS.submissionReview,
    PERMS.cfpManage,
    PERMS.scheduleEdit,
    PERMS.schedulePublish,
  );

  return (
    <PageContainer>
      <PageHeader
        title={`Hello${me?.display_name ? `, ${me.display_name}` : ''}`}
        description="Conventions, editions, calls for papers, and the live schedule."
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
                <ul className="space-y-1 text-sm">
                  {orgsQ.data.items.map((o) => (
                    <li key={o.id} className="flex items-baseline justify-between gap-2">
                      <span className="font-medium text-foreground">{o.display_name}</span>
                      {o.role ? (
                        <span className="text-xs text-muted-foreground">{o.role.replaceAll('_', ' ')}</span>
                      ) : null}
                    </li>
                  ))}
                </ul>
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
                description="You need a membership (or an events-* IdP group bound in access-config.toml). Dev seed is demo-org; ask an admin or check your groups."
              />
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <span className="rounded-pill bg-primary-soft p-1.5">
                <Inbox className="w-4 h-4 text-primary" />
              </span>
              <CardTitle>Programme</CardTitle>
            </div>
            <CardDescription>
              Calls for papers, event types, and the schedule builder live in the sidebar.
            </CardDescription>
          </CardHeader>
          <CardContent>
            {canProgramme ? (
              <div className="flex flex-wrap gap-2">
                {canAny(PERMS.submissionReview, PERMS.cfpManage) && (
                  <LinkButton to="/cfp" variant="secondary">
                    <Inbox className="w-3.5 h-3.5" />
                    Calls for papers
                  </LinkButton>
                )}
                {canAny(PERMS.cfpManage) && (
                  <LinkButton to="/event-types" variant="secondary">
                    <Shapes className="w-3.5 h-3.5" />
                    Event types
                  </LinkButton>
                )}
                {canAny(PERMS.scheduleEdit, PERMS.schedulePublish) && (
                  <LinkButton to="/schedule" variant="secondary">
                    <CalendarClock className="w-3.5 h-3.5" />
                    Schedule
                  </LinkButton>
                )}
              </div>
            ) : (
              <EmptyState
                icon={Inbox}
                title="No programme access"
                description="You need an events-admin, events-organizer, events-programme, or events-reviewer group (or a matching role assignment) to open CfPs and the schedule."
              />
            )}
          </CardContent>
        </Card>
      </div>
    </PageContainer>
  );
}
