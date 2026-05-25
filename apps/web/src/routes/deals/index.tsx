import { createRoute, Link } from '@tanstack/react-router';
import { useDeals } from '@/hooks/use-deals';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from '@/components/ui/table';
import { Route as rootRoute } from '../__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/deals',
  component: DealList,
});

function formatCurrency(amount: number | null, currency: string | null) {
  if (amount === null || !currency) return '-';
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency,
  }).format(amount);
}

function DealList() {
  const { data, isLoading, error } = useDeals();

  return (
    <div className="p-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Deals</h1>
      </div>

      {isLoading && (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      )}

      {error && (
        <p className="text-destructive">
          Failed to load deals: {error.message}
        </p>
      )}

      {data && data.data.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <p className="text-lg text-muted-foreground">No deals yet</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Deals will appear here once created via the API.
          </p>
        </div>
      )}

      {data && data.data.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Title</TableHead>
              <TableHead>Stage</TableHead>
              <TableHead>Amount</TableHead>
              <TableHead>Created</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.data.map((deal) => (
              <TableRow key={deal.id}>
                <TableCell>
                  <Link
                    to="/deals/$dealId"
                    params={{ dealId: deal.id }}
                    className="font-medium text-primary hover:underline"
                  >
                    {deal.title}
                  </Link>
                </TableCell>
                <TableCell>
                  {deal.stage_id ? (
                    <Badge variant="secondary">{deal.stage_id.slice(0, 8)}</Badge>
                  ) : (
                    <span className="text-muted-foreground">-</span>
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatCurrency(deal.amount, deal.currency)}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {new Date(deal.created_at).toLocaleDateString()}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
