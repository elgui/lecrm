import * as React from 'react';
import { createRoute, Link } from '@tanstack/react-router';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Plus } from 'lucide-react';
import { useDeals, useCreateDeal, useDealDefinitions } from '@/hooks/use-deals';
import { useBatchProperties } from '@/hooks/use-metadata-definitions';
import { usePipelineStages } from '@/hooks/use-pipeline-stages';
import { useMe } from '@/hooks/use-me';
import { formatPropertyValue } from '@/lib/format-property';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent } from '@/components/ui/card';
import { ExportButton } from '@/components/export-button';
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
  return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(amount);
}

const dealSchema = z.object({
  title: z.string().min(1, 'Title is required'),
  amount: z.string(),
  currency: z.string(),
  stage_id: z.string(),
  expected_close_date: z.string(),
});
type DealForm = z.infer<typeof dealSchema>;

function CreateDealForm({ onDone }: { onDone: () => void }) {
  const create = useCreateDeal();
  const { data: stagesResp } = usePipelineStages();
  const stages = stagesResp?.data;
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<DealForm>({
    resolver: zodResolver(dealSchema),
    defaultValues: { title: '', amount: '', currency: 'EUR', stage_id: '', expected_close_date: '' },
  });

  const onSubmit = handleSubmit((data) => {
    create.mutate(
      {
        title: data.title,
        amount: data.amount ? Number(data.amount) : null,
        currency: data.currency || null,
        stage_id: data.stage_id || null,
        contact_id: null,
        company_id: null,
        expected_close_date: data.expected_close_date || null,
      },
      { onSuccess: onDone },
    );
  });

  return (
    <Card className="mb-6">
      <CardContent className="pt-6">
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="title">Title</Label>
            <Input id="title" {...register('title')} />
            {errors.title && <p className="text-sm text-destructive">{errors.title.message}</p>}
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-2">
              <Label htmlFor="amount">Amount</Label>
              <Input id="amount" type="number" step="0.01" {...register('amount')} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="currency">Currency</Label>
              <Input id="currency" {...register('currency')} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="expected_close_date">Expected close</Label>
              <Input id="expected_close_date" type="date" {...register('expected_close_date')} />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="stage_id">Stage</Label>
            <select
              id="stage_id"
              {...register('stage_id')}
              className="h-10 w-full rounded-md border bg-background px-3 text-sm"
            >
              <option value="">—</option>
              {stages?.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </div>
          <div className="flex gap-2">
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? 'Creating…' : 'Create deal'}
            </Button>
            <Button type="button" variant="ghost" onClick={onDone}>
              Cancel
            </Button>
          </div>
          {create.isError && (
            <p className="text-sm text-destructive">{(create.error as Error).message}</p>
          )}
        </form>
      </CardContent>
    </Card>
  );
}

function DealList() {
  const { data, isLoading, error } = useDeals();
  const { data: stagesResp } = usePipelineStages();
  const stages = stagesResp?.data;
  const { permissions } = useMe();
  const [creating, setCreating] = React.useState(false);

  // Surface the workspace's first couple of custom fields as table columns so
  // tailorization is visible at a glance, not only on the detail page. Values
  // are batch-fetched for the whole page in one request (no N+1).
  const { data: defs } = useDealDefinitions();
  const customCols = (defs ?? []).slice(0, 2);
  // Only fetch values when there's at least one custom column to render.
  const dealIds = customCols.length > 0 ? (data?.data.map((d) => d.id) ?? []) : [];
  const { data: propsById } = useBatchProperties('deal', dealIds);

  const stageName = (id: string | null) =>
    stages?.find((s) => s.id === id)?.name ?? id?.slice(0, 8) ?? null;

  return (
    <div className="p-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Deals</h1>
        <div className="flex items-center gap-2">
          <ExportButton resource="deals" />
          {permissions.can_write && !creating && (
            <Button size="sm" onClick={() => setCreating(true)}>
              <Plus className="mr-2 h-4 w-4" />
              New deal
            </Button>
          )}
        </div>
      </div>

      {creating && <CreateDealForm onDone={() => setCreating(false)} />}

      {isLoading && (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      )}

      {error && <p className="text-destructive">Failed to load deals: {error.message}</p>}

      {data && data.data.length === 0 && !creating && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <p className="text-lg text-muted-foreground">No deals yet</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Create your first deal to get started.
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
              {customCols.map((def) => (
                <TableHead key={def.id}>{def.property_key}</TableHead>
              ))}
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
                    <Badge variant="secondary">{stageName(deal.stage_id)}</Badge>
                  ) : (
                    <span className="text-muted-foreground">-</span>
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatCurrency(deal.amount, deal.currency)}
                </TableCell>
                {customCols.map((def) => {
                  const formatted = formatPropertyValue(
                    def,
                    propsById?.[deal.id]?.[def.property_key],
                  );
                  return (
                    <TableCell key={def.id} className="text-muted-foreground">
                      {formatted || <span className="text-muted-foreground">-</span>}
                    </TableCell>
                  );
                })}
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
