import { useQuery } from '@tanstack/react-query';
import { Users2 } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { PageContainer, PageHeader } from '@/components/ui/PageHeader';
import { EmptyState, ErrorRow, LoadingRow } from '@/components/ui/States';
import { listOrganizations } from '@/api/organizations';

export function OrganizationsPage() {
  const orgsQ = useQuery({
    queryKey: ['organizations'],
    queryFn: ({ signal }) => listOrganizations(signal),
  });

  return (
    <PageContainer>
      <PageHeader
        title="Organizations"
        description="Every organisation you belong to and your role in it."
      />

      {orgsQ.isLoading ? (
        <LoadingRow />
      ) : orgsQ.isError ? (
        <ErrorRow message="Could not load organizations." />
      ) : !orgsQ.data || orgsQ.data.items.length === 0 ? (
        <EmptyState
          icon={Users2}
          title="You don't belong to any organisations yet"
          description="Ask an existing admin to invite you, or seed one through the migrations."
        />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {orgsQ.data.items.map((o) => (
            <Card key={o.id}>
              <CardHeader>
                <CardTitle className="text-base">{o.display_name}</CardTitle>
                <p className="text-xs text-muted-foreground font-mono">{o.slug}</p>
              </CardHeader>
              <CardContent>
                <p className="text-sm text-muted-foreground">
                  Role:{' '}
                  <span className="text-foreground font-semibold">
                    {o.role ?? 'member'}
                  </span>
                </p>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </PageContainer>
  );
}
