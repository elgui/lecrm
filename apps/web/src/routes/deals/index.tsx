import * as React from 'react';
import { createRoute, Link, useNavigate } from '@tanstack/react-router';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Plus, CircleDollarSign } from 'lucide-react';
import { useDeals, useCreateDeal, useDealDefinitions } from '@/hooks/use-deals';
import { useBatchProperties } from '@/hooks/use-metadata-definitions';
import { usePipelineStages } from '@/hooks/use-pipeline-stages';
import { useMe } from '@/hooks/use-me';
import { formatPropertyValue, customFieldLabel } from '@/lib/format-property';
import { stageBadgeVariant, formatDate } from '@/lib/format';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent } from '@/components/ui/card';
import { PageHeader } from '@/components/page-header';
import { EmptyState } from '@/components/empty-state';
import { ExportButton } from '@/components/export-button';
import { CsvImportWizard } from '@/components/csv-import-wizard';
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
  // `?new=true` (e.g. from the mobile create FAB) auto-opens the create form.
  validateSearch: (search: Record<string, unknown>): { new?: boolean } => ({
    new: search.new === true || search.new === 'true' ? true : undefined,
  }),
  component: DealList,
});

function formatCurrency(amount: number | null, currency: string | null) {
  if (amount === null || !currency) return '—';
  return new Intl.NumberFormat('fr-FR', { style: 'currency', currency }).format(amount);
}

const dealSchema = z.object({
  title: z.string().min(1, 'Le titre est requis'),
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
            <Label htmlFor="title">Titre</Label>
            <Input id="title" {...register('title')} />
            {errors.title && <p className="text-sm text-destructive">{errors.title.message}</p>}
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-2">
              <Label htmlFor="amount">Montant</Label>
              <Input id="amount" type="number" step="0.01" {...register('amount')} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="currency">Devise</Label>
              <Input id="currency" {...register('currency')} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="expected_close_date">Clôture prévue</Label>
              <Input id="expected_close_date" type="date" {...register('expected_close_date')} />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="stage_id">Étape</Label>
            <select
              id="stage_id"
              {...register('stage_id')}
              className="h-10 w-full rounded-md border border-input bg-card px-3 text-sm shadow-xs focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/25"
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
              {create.isPending ? 'Création…' : 'Créer l’affaire'}
            </Button>
            <Button type="button" variant="ghost" onClick={onDone}>
              Annuler
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
  const { new: openCreate } = Route.useSearch();
  const navigate = useNavigate();
  const [creating, setCreating] = React.useState(false);
  const [importing, setImporting] = React.useState(false);

  // Honour `?new=true` (mobile create FAB) once, then strip the param so a
  // refresh or back-nav doesn't keep re-opening the form.
  React.useEffect(() => {
    if (openCreate) {
      setCreating(true);
      navigate({ to: '/deals', search: {}, replace: true });
    }
  }, [openCreate, navigate]);

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

  const colSpan = 4 + customCols.length;

  return (
    <div className="mx-auto max-w-7xl p-4 md:p-8">
      <PageHeader
        title="Affaires"
        description="Gérez votre pipeline et votre chiffre d’affaires"
        actions={
          <>
            <ExportButton resource="deals" />
            {permissions.can_write && (
              <Button variant="outline" size="sm" onClick={() => setImporting(true)}>
                <Plus className="mr-1 h-4 w-4" />
                Importer CSV
              </Button>
            )}
            {permissions.can_write && !creating && (
              <Button onClick={() => setCreating(true)}>
                <Plus />
                Nouvelle affaire
              </Button>
            )}
          </>
        }
      />

      {importing && (
        <CsvImportWizard entity="deals" onClose={() => setImporting(false)} />
      )}

      {creating && <CreateDealForm onDone={() => setCreating(false)} />}

      {error && <p className="text-destructive">Échec du chargement des affaires : {error.message}</p>}

      <Card className="overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>Titre</TableHead>
              <TableHead>Étape</TableHead>
              <TableHead className="text-right">Montant</TableHead>
              {customCols.map((def) => (
                <TableHead key={def.id}>{customFieldLabel(def)}</TableHead>
              ))}
              <TableHead>Créé le</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              Array.from({ length: 6 }).map((_, i) => (
                <TableRow key={i} className="hover:bg-transparent">
                  <TableCell>
                    <Skeleton className="h-4 w-40" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-5 w-20 rounded-full" />
                  </TableCell>
                  <TableCell className="text-right">
                    <Skeleton className="ml-auto h-4 w-20" />
                  </TableCell>
                  {customCols.map((def) => (
                    <TableCell key={def.id}>
                      <Skeleton className="h-4 w-16" />
                    </TableCell>
                  ))}
                  <TableCell>
                    <Skeleton className="h-4 w-20" />
                  </TableCell>
                </TableRow>
              ))
            ) : !data || data.data.length === 0 ? (
              !creating && (
                <TableRow className="hover:bg-transparent">
                  <TableCell colSpan={colSpan} className="p-0">
                    <EmptyState
                      icon={CircleDollarSign}
                      title="Aucune affaire"
                      description="Créez votre première affaire pour commencer à suivre votre chiffre d’affaires."
                      action={
                        permissions.can_write && (
                          <Button onClick={() => setCreating(true)}>
                            <Plus />
                            Nouvelle affaire
                          </Button>
                        )
                      }
                    />
                  </TableCell>
                </TableRow>
              )
            ) : (
              data.data.map((deal) => {
                const name = stageName(deal.stage_id);
                return (
                  <TableRow key={deal.id} className="group">
                    <TableCell>
                      <Link
                        to="/deals/$dealId"
                        params={{ dealId: deal.id }}
                        className="font-medium text-primary group-hover:underline"
                      >
                        {deal.title}
                      </Link>
                    </TableCell>
                    <TableCell>
                      {deal.stage_id && name ? (
                        <Badge variant={stageBadgeVariant(name)}>{name}</Badge>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right font-medium tabular-nums">
                      {formatCurrency(deal.amount, deal.currency)}
                    </TableCell>
                    {customCols.map((def) => {
                      const formatted = formatPropertyValue(
                        def,
                        propsById?.[deal.id]?.[def.property_key],
                      );
                      return (
                        <TableCell key={def.id} className="text-muted-foreground">
                          {formatted || <span className="text-muted-foreground">—</span>}
                        </TableCell>
                      );
                    })}
                    <TableCell className="text-muted-foreground tabular-nums">
                      {formatDate(deal.created_at)}
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
        {!isLoading && data && data.data.length > 0 && (
          <div className="flex items-center justify-between border-t border-border px-4 py-2.5 text-xs text-muted-foreground">
            <span>
              {data.data.length} {data.data.length === 1 ? 'affaire' : 'affaires'}
            </span>
          </div>
        )}
      </Card>
    </div>
  );
}
