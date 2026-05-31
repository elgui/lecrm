import { createRoute, Link } from '@tanstack/react-router';
import { Users, Building2, CircleDollarSign, ArrowRight } from 'lucide-react';
import { Card } from '@/components/ui/card';
import { PageHeader } from '@/components/page-header';
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
    description: 'Manage your contacts and relationships',
    tone: 'bg-blue-50 text-blue-600',
  },
  {
    to: '/companies' as const,
    icon: Building2,
    title: 'Companies',
    description: 'Track organizations and accounts',
    tone: 'bg-violet-50 text-violet-600',
  },
  {
    to: '/deals' as const,
    icon: CircleDollarSign,
    title: 'Deals',
    description: 'Manage your pipeline and revenue',
    tone: 'bg-emerald-50 text-emerald-600',
  },
];

function Dashboard() {
  return (
    <div className="mx-auto max-w-7xl p-8">
      <PageHeader
        title="Dashboard"
        description="A quick overview of your workspace."
      />
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
