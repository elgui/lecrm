import { createRoute, Link, useNavigate } from '@tanstack/react-router';
import { useForm } from 'react-hook-form';
import {
  useDeal,
  useUpdateDeal,
  useDeleteDeal,
  useDealProperties,
  useUpdateDealProperties,
  useDealDefinitions,
} from '@/hooks/use-deals';
import { usePipelineStages } from '@/hooks/use-pipeline-stages';
import { useCompany } from '@/hooks/use-companies';
import { useContact } from '@/hooks/use-contacts';
import { useMe } from '@/hooks/use-me';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Badge } from '@/components/ui/badge';
import { stageBadgeVariant } from '@/lib/format';
import { ArrowLeft, Trash2 } from 'lucide-react';
import { NotesPanel } from '@/components/notes-panel';
import { TasksPanel } from '@/components/tasks-panel';
import { CustomPropertiesFields } from '@/components/custom-properties-editor';
import { RecordSaveBar } from '@/components/record-save-bar';
import { useCustomPropertyForm } from '@/hooks/use-custom-property-form';
import { AssistantIaRail } from '@/components/assistant-ia-rail';
import { Route as rootRoute } from '../__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/deals/$dealId',
  component: DealDetail,
});

interface DealFormData {
  title: string;
  amount: string;
  currency: string;
  stage_id: string;
  expected_close_date: string;
}

function DealDetail() {
  const { dealId } = Route.useParams();
  const navigate = useNavigate();
  const { data: deal, isLoading } = useDeal(dealId);
  const { data: stagesResp } = usePipelineStages();
  const stages = stagesResp?.data;
  const { data: company } = useCompany(deal?.company_id ?? '');
  const { data: primaryContact } = useContact(deal?.contact_id ?? '');
  const { data: properties, isLoading: propsLoading } = useDealProperties(dealId);
  const { data: definitions } = useDealDefinitions();
  const updateMutation = useUpdateDeal(dealId);
  const updateProps = useUpdateDealProperties(dealId);
  const deleteMutation = useDeleteDeal();
  const { permissions } = useMe();
  const canWrite = permissions.can_write;

  const form = useForm<DealFormData>({
    values: deal
      ? {
          title: deal.title,
          amount: deal.amount !== null ? String(deal.amount) : '',
          currency: deal.currency ?? '',
          stage_id: deal.stage_id ?? '',
          expected_close_date: deal.expected_close_date ?? '',
        }
      : undefined,
  });

  const customProps = useCustomPropertyForm(definitions, properties);

  // Single save: persist core fields and custom properties together (see the
  // contact detail page for the rationale — no more two-button data-loss trap).
  const coreDirty = form.formState.isDirty;
  const anyDirty = coreDirty || customProps.isDirty;
  const isSaving = updateMutation.isPending || updateProps.isPending;
  const saveError = updateProps.isError
    ? (updateProps.error as Error).message
    : updateMutation.isError
      ? (updateMutation.error as Error).message
      : null;

  const onSaveAll = async () => {
    let coreOk = true;
    if (coreDirty) {
      coreOk = false;
      await form.handleSubmit((data) => {
        coreOk = true;
        updateMutation.mutate({
          title: data.title,
          amount: data.amount ? Number(data.amount) : null,
          currency: data.currency || null,
          stage_id: data.stage_id || null,
          expected_close_date: data.expected_close_date || null,
        });
      })();
    }
    if (coreOk && customProps.isDirty) {
      updateProps.mutate(customProps.buildPayload());
    }
  };

  const onDelete = () => {
    if (!window.confirm('Supprimer cette affaire ? Cette action est irréversible.')) return;
    deleteMutation.mutate(dealId, { onSuccess: () => navigate({ to: '/deals' }) });
  };

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
        <p className="text-destructive">Affaire introuvable</p>
      </div>
    );
  }

  const stageNameValue =
    stages?.find((s) => s.id === deal.stage_id)?.name ?? null;
  const amountLabel =
    deal.amount !== null && deal.currency
      ? new Intl.NumberFormat('fr-FR', {
          style: 'currency',
          currency: deal.currency,
        }).format(deal.amount)
      : null;

  return (
    <div className="mx-auto max-w-5xl p-8">
      <AssistantIaRail recordKind="deal" recordName={deal.title} />
      <Link
        to="/deals"
        className="mb-4 inline-flex items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Retour aux affaires
      </Link>
      <div className="mb-6 flex items-start justify-between gap-4">
        <div className="space-y-2">
          <h1 className="text-xl font-semibold tracking-tight">{deal.title}</h1>
          <div className="flex items-center gap-2">
            {stageNameValue && (
              <Badge variant={stageBadgeVariant(stageNameValue)}>
                {stageNameValue}
              </Badge>
            )}
            {amountLabel && (
              <span className="text-sm font-medium tabular-nums text-muted-foreground">
                {amountLabel}
              </span>
            )}
          </div>
          {(company || primaryContact) && (
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
              {company && (
                <span>
                  Entreprise :{' '}
                  <Link
                    to="/companies/$companyId"
                    params={{ companyId: company.id }}
                    className="font-medium text-primary hover:underline"
                  >
                    {company.name}
                  </Link>
                </span>
              )}
              {primaryContact && (
                <span>
                  Contact :{' '}
                  <Link
                    to="/contacts/$contactId"
                    params={{ contactId: primaryContact.id }}
                    className="font-medium text-primary hover:underline"
                  >
                    {`${primaryContact.first_name ?? ''} ${primaryContact.last_name ?? ''}`.trim() ||
                      primaryContact.email ||
                      'Inconnu'}
                  </Link>
                </span>
              )}
            </div>
          )}
        </div>
        {canWrite && (
          <Button variant="outline" size="sm" onClick={onDelete} disabled={deleteMutation.isPending}>
            <Trash2 className="mr-2 h-4 w-4" />
            Supprimer
          </Button>
        )}
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Détails de l’affaire</CardTitle>
          </CardHeader>
          <CardContent>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void onSaveAll();
              }}
              className="space-y-4"
            >
              <div className="space-y-2">
                <Label htmlFor="title">Titre</Label>
                <Input id="title" readOnly={!canWrite} {...form.register('title', { required: true })} />
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="amount">Montant</Label>
                  <Input id="amount" type="number" step="0.01" readOnly={!canWrite} {...form.register('amount')} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="currency">Devise</Label>
                  <Input id="currency" readOnly={!canWrite} {...form.register('currency')} />
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="stage_id">Étape</Label>
                <select
                  id="stage_id"
                  disabled={!canWrite}
                  {...form.register('stage_id')}
                  className="h-10 w-full rounded-md border border-input bg-card px-3 text-sm shadow-xs focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/25 disabled:opacity-50"
                >
                  <option value="">—</option>
                  {stages?.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="expected_close_date">Date de clôture prévue</Label>
                <Input
                  id="expected_close_date"
                  type="date"
                  readOnly={!canWrite}
                  {...form.register('expected_close_date')}
                />
              </div>
              {/* Submit on Enter; the page-level RecordSaveBar is the
                  primary, single save action for core + custom fields. */}
              <button type="submit" className="hidden" aria-hidden tabIndex={-1} />
            </form>
          </CardContent>
        </Card>

        <CustomPropertiesFields
          definitions={definitions}
          form={customProps.form}
          onChange={customProps.set}
          isLoading={propsLoading}
          canWrite={canWrite}
        />

        <RecordSaveBar
          className="lg:col-span-2"
          canWrite={canWrite}
          isDirty={anyDirty}
          isSaving={isSaving}
          isSuccess={updateMutation.isSuccess || updateProps.isSuccess}
          error={saveError}
          onSave={() => void onSaveAll()}
        />

        <NotesPanel entityType="deal" entityId={dealId} />
        <TasksPanel scope={{ entity_type: 'deal', entity_id: dealId }} />
      </div>
    </div>
  );
}
