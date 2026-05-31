import { createRoute, Link } from '@tanstack/react-router';
import {
  Users,
  Building2,
  CircleDollarSign,
  TrendingUp,
  ListChecks,
  CalendarClock,
  ArrowRight,
} from 'lucide-react';
import { Card } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { PageHeader } from '@/components/page-header';
import { useContacts } from '@/hooks/use-contacts';
import { useCompanies } from '@/hooks/use-companies';
import { useDeals } from '@/hooks/use-deals';
import { useTasks } from '@/hooks/use-tasks';
import { computeDealStats } from '@/lib/dashboard-stats';
import {
  selectAttentionTasks,
  selectClosingDeals,
  relativeDayLabel,
  type AttentionTask,
  type ClosingDeal,
} from '@/lib/attention';
import { formatAmount } from '@/lib/format';
import type { Company, EntityType, PaginatedResponse } from '@/lib/types';
import { Route as rootRoute } from './__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: Dashboard,
});

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
        <Skeleton className="mt-1.5 h-9 w-24" />
      ) : (
        <p className="mt-1.5 text-3xl font-semibold tracking-tight tabular-nums text-foreground">
          {value}
        </p>
      )}
    </Card>
  );
}

/** Badge tone for a relative-day offset: overdue → red, due now/soon → amber. */
function dueTone(days: number | null): 'destructive' | 'warning' | 'secondary' {
  if (days === null) return 'secondary';
  if (days < 0) return 'destructive';
  if (days <= 2) return 'warning';
  return 'secondary';
}

/** Detail-route link props for a task's linked record, or null when unlinked. */
function entityRoute(type: EntityType, id: string) {
  switch (type) {
    case 'deal':
      return { to: '/deals/$dealId', params: { dealId: id } } as const;
    case 'contact':
      return { to: '/contacts/$contactId', params: { contactId: id } } as const;
    case 'company':
      return { to: '/companies/$companyId', params: { companyId: id } } as const;
  }
}

function TaskRow({ item }: { item: AttentionTask }) {
  const { task, daysUntilDue, overdue } = item;
  const route =
    task.entity_type && task.entity_id
      ? entityRoute(task.entity_type, task.entity_id)
      : null;

  const title = route ? (
    <Link
      {...route}
      className="font-medium text-foreground hover:text-primary hover:underline"
    >
      {task.title}
    </Link>
  ) : (
    <span className="font-medium text-foreground">{task.title}</span>
  );

  return (
    <li className="flex items-center justify-between gap-3 py-2.5">
      <div className="min-w-0">
        <p className="truncate text-sm">{title}</p>
        {task.description && (
          <p className="truncate text-xs text-muted-foreground">
            {task.description}
          </p>
        )}
      </div>
      {daysUntilDue === null ? (
        <span className="shrink-0 text-xs text-muted-foreground">
          Sans échéance
        </span>
      ) : (
        <Badge variant={dueTone(daysUntilDue)} className="shrink-0">
          {overdue && <span aria-hidden>•</span>}
          {relativeDayLabel(daysUntilDue)}
        </Badge>
      )}
    </li>
  );
}

function DealRow({
  item,
  companyName,
}: {
  item: ClosingDeal;
  companyName: string | null;
}) {
  const { deal, daysUntilClose, overdue } = item;
  return (
    <li className="flex items-center justify-between gap-3 py-2.5">
      <div className="min-w-0">
        <Link
          to="/deals/$dealId"
          params={{ dealId: deal.id }}
          className="block truncate text-sm font-medium text-foreground hover:text-primary hover:underline"
        >
          {deal.title}
        </Link>
        <p className="truncate text-xs text-muted-foreground">
          {companyName ?? 'Sans entreprise'}
          {deal.amount !== null && (
            <>
              {' · '}
              <span className="tabular-nums">
                {formatAmount(deal.amount, deal.currency ?? 'EUR')}
              </span>
            </>
          )}
        </p>
      </div>
      <Badge variant={dueTone(daysUntilClose)} className="shrink-0">
        {overdue && <span aria-hidden>•</span>}
        {relativeDayLabel(daysUntilClose)}
      </Badge>
    </li>
  );
}

/** Calm panel wrapper: titled card with an icon chip and a footer link. */
function AttentionPanel({
  icon: Icon,
  tone,
  title,
  loading,
  empty,
  emptyLabel,
  footer,
  children,
}: {
  icon: typeof ListChecks;
  tone: string;
  title: string;
  loading: boolean;
  empty: boolean;
  emptyLabel: string;
  footer: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <Card className="flex flex-col p-5">
      <div className="flex items-center gap-3">
        <div
          className={`flex h-9 w-9 items-center justify-center rounded-lg ${tone}`}
        >
          <Icon className="h-[18px] w-[18px]" />
        </div>
        <h2 className="text-base font-semibold text-foreground">{title}</h2>
      </div>

      <div className="mt-2 flex-1">
        {loading ? (
          <div className="space-y-3 py-2">
            <Skeleton className="h-5 w-full" />
            <Skeleton className="h-5 w-4/5" />
            <Skeleton className="h-5 w-3/5" />
          </div>
        ) : empty ? (
          <p className="py-6 text-sm text-muted-foreground">{emptyLabel}</p>
        ) : (
          <ul className="divide-y divide-border">{children}</ul>
        )}
      </div>

      <div className="mt-3 border-t border-border pt-3">{footer}</div>
    </Card>
  );
}

function Dashboard() {
  const { data: contacts } = useContacts();
  const { data: companies } = useCompanies();
  const { data: deals, isLoading: dealsLoading } = useDeals();
  const { data: tasks, isLoading: tasksLoading } = useTasks();

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

  // `now` is read once per render; the helpers take it explicitly so they stay
  // pure and unit-testable.
  const now = new Date();
  const attentionTasks = selectAttentionTasks(tasks ?? [], now);
  const closingDeals = selectClosingDeals(deals?.data ?? [], now);

  const companyNames = new Map<string, string>(
    (companies?.data ?? []).map((c: Company) => [c.id, c.name]),
  );

  return (
    <div className="mx-auto max-w-7xl p-8">
      <PageHeader
        title="Tableau de bord"
        description="Ce qui demande votre attention aujourd’hui."
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

      <div className="grid gap-4 md:grid-cols-2">
        <AttentionPanel
          icon={ListChecks}
          tone="bg-blue-50 text-blue-600"
          title="Tâches à suivre"
          loading={tasksLoading}
          empty={attentionTasks.length === 0}
          emptyLabel="Aucune tâche en attente — tout est à jour."
          footer={
            <Link
              to="/tasks"
              className="group inline-flex items-center gap-1 text-sm font-medium text-primary"
            >
              Voir toutes les tâches
              <ArrowRight className="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
            </Link>
          }
        >
          {attentionTasks.map((item) => (
            <TaskRow key={item.task.id} item={item} />
          ))}
        </AttentionPanel>

        <AttentionPanel
          icon={CalendarClock}
          tone="bg-emerald-50 text-emerald-600"
          title="Affaires à conclure (14 j)"
          loading={dealsLoading}
          empty={closingDeals.length === 0}
          emptyLabel="Aucune affaire à conclure dans les 14 prochains jours."
          footer={
            <Link
              to="/deals"
              className="group inline-flex items-center gap-1 text-sm font-medium text-primary"
            >
              Voir toutes les affaires
              <ArrowRight className="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
            </Link>
          }
        >
          {closingDeals.map((item) => (
            <DealRow
              key={item.deal.id}
              item={item}
              companyName={
                item.deal.company_id
                  ? companyNames.get(item.deal.company_id) ?? null
                  : null
              }
            />
          ))}
        </AttentionPanel>
      </div>
    </div>
  );
}
