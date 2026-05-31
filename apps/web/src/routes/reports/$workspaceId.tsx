import * as React from 'react';
import { createRoute } from '@tanstack/react-router';
import { BarChart3 } from 'lucide-react';

import { Route as rootRoute } from '../__root';
import { useAuth } from '@/hooks/use-auth';
import { useEmbedToken } from '@/hooks/use-embed-token';
import { useDeals } from '@/hooks/use-deals';
import { ApiError } from '@/lib/api';
import { BASELINE_DASHBOARDS, type DashboardSpec } from '@/lib/reports';
import { CubeFrame } from '@/components/reports/cube-frame';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { EmptyState } from '@/components/empty-state';
import { cn } from '@/lib/utils';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/reports/$workspaceId',
  component: ReportsPage,
});

function describeEmbedError(error: Error): string {
  if (error instanceof ApiError) {
    if (error.status === 401) return 'Your session has expired. Please sign in again.';
    if (error.status === 403) return 'You do not have permission to view reports for this workspace.';
    if (error.status === 503) return 'Embedded reporting is not configured on this server.';
    return `Embed token request failed (${error.status}).`;
  }
  return 'Could not reach the reporting service.';
}

function ReportsPage() {
  const { workspaceId } = Route.useParams();
  const auth = useAuth();
  const tokenQuery = useEmbedToken();
  const dealsQuery = useDeals();

  const [activeId, setActiveId] = React.useState<string>(
    BASELINE_DASHBOARDS[0]!.id,
  );
  const active = BASELINE_DASHBOARDS.find((d) => d.id === activeId)
    ?? BASELINE_DASHBOARDS[0]!;

  // Defense in depth: surface a mismatch instead of silently trusting
  // whatever the URL says. The backend will still reject a wrong
  // workspace on the embed-token call (workspace context comes from
  // the subdomain).
  const workspaceMismatch =
    !!auth.user && !!workspaceId && auth.user.workspace_id !== workspaceId;

  return (
    <div className="p-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight">Reports</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Dashboards for your pipeline and activity.
        </p>
      </div>

      {auth.isLoading && (
        <div className="space-y-3">
          <Skeleton className="h-10 w-64" />
          <Skeleton className="h-96 w-full" />
        </div>
      )}

      {workspaceMismatch && (
        <Card>
          <CardContent className="py-8 text-center">
            <p className="text-destructive">
              Workspace mismatch — this URL is for a different workspace
              than you are signed into.
            </p>
          </CardContent>
        </Card>
      )}

      {!auth.isLoading && !workspaceMismatch && (
        <ReportsBody
          tokenQuery={tokenQuery}
          dealsQuery={dealsQuery}
          active={active}
          setActiveId={setActiveId}
        />
      )}
    </div>
  );
}

interface ReportsBodyProps {
  tokenQuery: ReturnType<typeof useEmbedToken>;
  dealsQuery: ReturnType<typeof useDeals>;
  active: DashboardSpec;
  setActiveId: (id: string) => void;
}

export function ReportsBody({
  tokenQuery,
  dealsQuery,
  active,
  setActiveId,
}: ReportsBodyProps) {
  // Empty-state pre-check: a workspace with zero deals will render
  // empty charts. Surface that explicitly so the iframe never appears
  // broken. Activities aren't surfaced via the v0 REST API yet, so we
  // gate on deals only — good enough for the v0 baseline.
  const dealsLoaded = !!dealsQuery.data;
  const hasNoDeals = dealsLoaded && dealsQuery.data!.data.length === 0;

  if (hasNoDeals && !dealsQuery.isLoading) {
    return (
      <Card>
        <EmptyState
          icon={BarChart3}
          title="No data to report yet"
          description="Create your first deal to see dashboards populated here."
        />
      </Card>
    );
  }

  if (tokenQuery.isLoading || dealsQuery.isLoading) {
    return (
      <div className="space-y-4" data-testid="reports-loading">
        <Skeleton className="h-10 w-64" />
        <Skeleton className="h-96 w-full" />
      </div>
    );
  }

  if (tokenQuery.error) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-lg text-destructive">
            Reports unavailable
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            {describeEmbedError(tokenQuery.error)}
          </p>
        </CardContent>
      </Card>
    );
  }

  if (!tokenQuery.data) return null;

  return (
    <div className="space-y-4">
      <DashboardTabs activeId={active.id} onSelect={setActiveId} />
      <div>
        <h2 className="mb-1 text-lg font-medium">{active.title}</h2>
        <p className="mb-3 text-sm text-muted-foreground">
          {active.description}
        </p>
        <CubeFrame token={tokenQuery.data.token} dashboard={active} />
      </div>
    </div>
  );
}

interface DashboardTabsProps {
  activeId: string;
  onSelect: (id: string) => void;
}

function DashboardTabs({ activeId, onSelect }: DashboardTabsProps) {
  return (
    <div role="tablist" className="flex gap-2 border-b">
      {BASELINE_DASHBOARDS.map((d) => {
        const isActive = d.id === activeId;
        return (
          <button
            key={d.id}
            role="tab"
            aria-selected={isActive}
            onClick={() => onSelect(d.id)}
            className={cn(
              '-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors',
              isActive
                ? 'border-primary text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground',
            )}
          >
            {d.title}
          </button>
        );
      })}
    </div>
  );
}
