import { createRoute } from '@tanstack/react-router';
import { BarChart3, LineChart, Sparkles } from 'lucide-react';

import { Route as rootRoute } from '../__root';
import { useEmbedToken } from '@/hooks/use-embed-token';
import { useDeals } from '@/hooks/use-deals';
import {
  BASELINE_DASHBOARDS,
  type DashboardSpec,
} from '@/lib/reports';
import { CubeFrame } from '@/components/reports/cube-frame';
import { ReportsWorkspace } from '@/components/reports/reports-workspace';
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

// Native reporting is the live path on every deployment (incl. the demo): it
// runs aggregation SQL directly against the workspace schema, so it never
// depends on the Cube.dev embed stack being provisioned. The Cube iframe path
// (ReportsBody / CubeFrame below) is retained for deployments that wire Cube,
// and is still covered by reports-body.test.tsx.
function ReportsPage() {
  return (
    <div className="p-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight">Rapports</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Construisez vos indicateurs : un indicateur, un regroupement, une
          période — avec comparaison à N-1 (année précédente).
        </p>
      </div>
      <ReportsWorkspace />
    </div>
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
                <CardTitle className="text-lg">Rapports</CardTitle>
                <Badge variant="secondary" className="gap-1">
                  <Sparkles className="h-3 w-3" />
                  Bientôt disponible
                </Badge>
              </div>
              <p className="mt-1 text-sm text-muted-foreground">
                Les tableaux de bord en temps réel de votre pipeline et de
                votre activité arrivent bientôt.
              </p>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <p className="mb-3 text-sm font-medium text-foreground">
            Ce que vous pourrez suivre :
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
          title="Aucune donnée à afficher pour le moment"
          description="Créez votre première affaire pour voir les tableaux de bord se remplir ici."
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

  // A token/config failure must never dead-end the page (especially mid-demo).
  // Fall back to the same honest branded placeholder we show when reporting is
  // not yet provisioned, rather than a red English error card.
  if (tokenQuery.error) {
    return <ReportsComingSoon />;
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
