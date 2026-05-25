import { createRoute, Link } from '@tanstack/react-router';
import { useDeal } from '@/hooks/use-deals';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { ArrowLeft } from 'lucide-react';
import { Route as rootRoute } from '../__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/deals/$dealId',
  component: DealDetail,
});

function DealDetail() {
  const { dealId } = Route.useParams();
  const { data: deal, isLoading } = useDeal(dealId);

  if (isLoading) {
    return (
      <div className="space-y-4 p-8">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (!deal) {
    return (
      <div className="p-8">
        <p className="text-destructive">Deal not found</p>
      </div>
    );
  }

  return (
    <div className="p-8">
      <div className="mb-6">
        <Link
          to="/deals"
          className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to deals
        </Link>
        <h1 className="text-2xl font-semibold">{deal.title}</h1>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Deal Details</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-4 sm:grid-cols-2">
            {deal.stage_id && (
              <div>
                <dt className="text-sm font-medium text-muted-foreground">Stage</dt>
                <dd className="mt-1">
                  <Badge variant="secondary">{deal.stage_id.slice(0, 8)}</Badge>
                </dd>
              </div>
            )}
            <div>
              <dt className="text-sm font-medium text-muted-foreground">Amount</dt>
              <dd className="mt-1 text-sm">
                {deal.amount !== null && deal.currency
                  ? new Intl.NumberFormat(undefined, {
                      style: 'currency',
                      currency: deal.currency,
                    }).format(deal.amount)
                  : '-'}
              </dd>
            </div>
            {deal.currency && (
              <div>
                <dt className="text-sm font-medium text-muted-foreground">Currency</dt>
                <dd className="mt-1 text-sm">{deal.currency}</dd>
              </div>
            )}
            <div>
              <dt className="text-sm font-medium text-muted-foreground">Created</dt>
              <dd className="mt-1 text-sm">
                {new Date(deal.created_at).toLocaleDateString()}
              </dd>
            </div>
            {deal.closed_at && (
              <div>
                <dt className="text-sm font-medium text-muted-foreground">Closed</dt>
                <dd className="mt-1 text-sm">
                  {new Date(deal.closed_at).toLocaleDateString()}
                </dd>
              </div>
            )}
          </dl>
        </CardContent>
      </Card>
    </div>
  );
}
