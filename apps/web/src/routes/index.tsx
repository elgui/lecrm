import { createRoute, Link } from '@tanstack/react-router';
import {
  Users,
  Building2,
  CircleDollarSign,
  TrendingUp,
  ArrowRight,
} from 'lucide-react';
import { Card } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { PageHeader } from '@/components/page-header';
import { useContacts } from '@/hooks/use-contacts';
import { useCompanies } from '@/hooks/use-companies';
import { useDeals } from '@/hooks/use-deals';
import { computeDealStats } from '@/lib/dashboard-stats';
import { formatAmount } from '@/lib/format';
import type { PaginatedResponse } from '@/lib/types';
import { Route as rootRoute } from './__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: Dashboard,
});

const TILES = [
  {
    to: '/contacts' as const,
    icon: Users,
    title: 'Contacts',
    description: 'Gérez vos contacts et vos relations',
    tone: 'bg-blue-50 text-blue-600',
  },
  {
    to: '/companies' as const,
    icon: Building2,
    title: 'Entreprises',
    description: 'Suivez vos organisations et vos comptes',
    tone: 'bg-violet-50 text-violet-600',
  },
  {
    to: '/deals' as const,
    icon: CircleDollarSign,
    title: 'Affaires',
    description: 'Gérez votre pipeline et votre chiffre d’affaires',
    tone: 'bg-emerald-50 text-emerald-600',
  },
];

/**
 * Render a count from a paginated list response. When the API signals more
 * pages (`has_more`), the loaded length is a lower bound, so we suffix "+"
 * rather than report a misleadingly exact number.
 */
function countLabel(resp: PaginatedResponse<unknown> | undefined): string | null {
  if (!resp) return null;
  return `${resp.data.length}${resp.has_more ? '+' : ''}`;
}

function StatCard({
  icon: Icon,
  tone,
  label,
  value,
}: {
  icon: typeof Users;
  tone: string;
  label: string;
  value: string | null;
}) {
  return (
    <Card className="p-5">
      <div
        className={`flex h-10 w-10 items-center justify-center rounded-lg ${tone}`}
      >
        <Icon className="h-5 w-5" />
      </div>
      <p className="mt-4 text-sm text-muted-foreground">{label}</p>
      {value === null ? (
        <Skeleton className="mt-1 h-8 w-20" />
      ) : (
        <p className="mt-1 text-2xl font-semibold tracking-tight tabular-nums text-foreground">
          {value}
        </p>
      )}
    </Card>
  );
}

function Dashboard() {
  const { data: contacts } = useContacts();
  const { data: companies } = useCompanies();
  const { data: deals } = useDeals();

  const dealStats = deals ? computeDealStats(deals.data) : null;
  const openDealsLabel =
    dealStats === null
      ? null
      : `${dealStats.openCount}${deals?.has_more ? '+' : ''}`;
  const pipelineLabel =
    dealStats === null
      ? null
      : // computeDealStats only sees the loaded page, so when more pages exist
        // the sum is a lower bound — suffix "+" to match the open-deals count.
        `${formatAmount(dealStats.openValue, dealStats.currency)}${
          deals?.has_more ? '+' : ''
        }`;

  return (
    <div className="mx-auto max-w-7xl p-8">
      <PageHeader
        title="Tableau de bord"
        description="Un aperçu rapide de votre espace de travail."
      />

      <div className="mb-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          icon={Users}
          tone="bg-blue-50 text-blue-600"
          label="Contacts"
          value={countLabel(contacts)}
        />
        <StatCard
          icon={Building2}
          tone="bg-violet-50 text-violet-600"
          label="Entreprises"
          value={countLabel(companies)}
        />
        <StatCard
          icon={CircleDollarSign}
          tone="bg-emerald-50 text-emerald-600"
          label="Affaires en cours"
          value={openDealsLabel}
        />
        <StatCard
          icon={TrendingUp}
          tone="bg-amber-50 text-amber-600"
          label="Pipeline en cours"
          value={pipelineLabel}
        />
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        {TILES.map(({ to, icon: Icon, title, description, tone }) => (
          <Link key={to} to={to} className="group">
            <Card className="h-full p-5 transition-all hover:-translate-y-0.5 hover:shadow-card-hover">
              <div className="flex items-start justify-between">
                <div
                  className={`flex h-10 w-10 items-center justify-center rounded-lg ${tone}`}
                >
                  <Icon className="h-5 w-5" />
                </div>
                <ArrowRight className="h-4 w-4 text-muted-foreground/40 transition-colors group-hover:text-primary" />
              </div>
              <h3 className="mt-4 text-base font-semibold text-foreground">
                {title}
              </h3>
              <p className="mt-1 text-sm text-muted-foreground">{description}</p>
            </Card>
          </Link>
        ))}
      </div>
    </div>
  );
}
