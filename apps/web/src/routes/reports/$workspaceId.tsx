import * as React from 'react';
import { createRoute } from '@tanstack/react-router';
import { BarChart3, LineChart, Sparkles } from 'lucide-react';

import { Route as rootRoute } from '../__root';
import { useAuth } from '@/hooks/use-auth';
import { useEmbedToken } from '@/hooks/use-embed-token';
import { useDeals } from '@/hooks/use-deals';
import { ApiError } from '@/lib/api';
import {
  BASELINE_DASHBOARDS,
  reportsEnabled,
  type DashboardSpec,
} from '@/lib/reports';
import { CubeFrame } from '@/components/reports/cube-frame';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
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
  // Embedded reporting (Cube.dev) is not provisioned on every
  // deployment — notably not on the public demo. Rather than let the
  // embed-token call 503 into a red "not configured" error mid-demo,
  // render an honest branded placeholder until the stack is wired up.
  // See reportsEnabled() in @/lib/reports for what "wired up" means.
  if (!reportsEnabled()) {
    return (
      <div className="p-8">
        <ReportsComingSoon />
      </div>
    );
  }

  return (
    <div className="p-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight">Reports</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Dashboards for your pipeline and activity.
        </p>
      </div>
      <ReportsLive />
    </div>
  );
}

// Live reporting path — gated behind reportsEnabled(). Holds the data
// hooks so they (and their network calls / audit writes) never fire
// when reporting is disabled.
function ReportsLive() {
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

  if (auth.isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-10 w-64" />
        <Skeleton className="h-96 w-full" />
      </div>
    );
  }

  if (workspaceMismatch) {
    return (
      <Card>
        <CardContent className="py-8 text-center">
          <p className="text-destructive">
            Workspace mismatch — this URL is for a different workspace
            than you are signed into.
          </p>
        </CardContent>
      </Card>
    );
  }

  return (
    <ReportsBody
      tokenQuery={tokenQuery}
      dealsQuery={dealsQuery}
      active={active}
      setActiveId={setActiveId}
    />
  );
}

// Honest, branded placeholder shown when embedded reporting is not yet
// provisioned. Same spirit as the AI-seat placeholder: no fake charts,
// no error styling — just a clear "what's coming" preview built from
// the real baseline dashboard catalogue so it reads as a roadmap, not
// a dead end.
export function ReportsComingSoon() {
  return (
    <div className="mx-auto max-w-2xl">
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-11 w-11 items-center justify-center rounded-full bg-primary/10 text-primary">
              <BarChart3 className="h-5 w-5" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <CardTitle className="text-lg">Reports</CardTitle>
                <Badge variant="secondary" className="gap-1">
                  <Sparkles className="h-3 w-3" />
                  Coming soon
                </Badge>
              </div>
              <p className="mt-1 text-sm text-muted-foreground">
                Live dashboards over your pipeline and activity are on
                the way.
              </p>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <p className="mb-3 text-sm font-medium text-foreground">
            What you'll be able to track:
          </p>
          <ul className="space-y-2.5">
            {BASELINE_DASHBOARDS.map((d) => (
              <li key={d.id} className="flex items-start gap-3">
                <LineChart className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                <div>
                  <p className="text-sm font-medium text-foreground">
                    {d.title}
                  </p>
                  <p className="text-sm text-muted-foreground">
                    {d.description}
                  </p>
                </div>
              </li>
            ))}
          </ul>
        </CardContent>
      </Card>
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
