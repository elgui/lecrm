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
import { useMe } from '@/hooks/use-me';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { ArrowLeft, Trash2 } from 'lucide-react';
import { NotesPanel } from '@/components/notes-panel';
import { TasksPanel } from '@/components/tasks-panel';
import { CustomPropertiesEditor } from '@/components/custom-properties-editor';
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

  const onSubmit = form.handleSubmit((data) => {
    updateMutation.mutate({
      title: data.title,
      amount: data.amount ? Number(data.amount) : null,
      currency: data.currency || null,
      stage_id: data.stage_id || null,
      expected_close_date: data.expected_close_date || null,
    });
  });

  const onDelete = () => {
    if (!window.confirm('Delete this deal? This cannot be undone.')) return;
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
        <p className="text-destructive">Deal not found</p>
      </div>
    );
  }

  return (
    <div className="p-8">
      <div className="mb-6 flex items-start justify-between">
        <div>
          <Link
            to="/deals"
            className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
          >
            <ArrowLeft className="h-4 w-4" />
            Back to deals
          </Link>
          <h1 className="text-2xl font-semibold">{deal.title}</h1>
        </div>
        {canWrite && (
          <Button variant="outline" size="sm" onClick={onDelete} disabled={deleteMutation.isPending}>
            <Trash2 className="mr-2 h-4 w-4" />
            Delete
          </Button>
        )}
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Deal Details</CardTitle>
          </CardHeader>
          <CardContent>
            <form onSubmit={onSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="title">Title</Label>
                <Input id="title" readOnly={!canWrite} {...form.register('title', { required: true })} />
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="amount">Amount</Label>
                  <Input id="amount" type="number" step="0.01" readOnly={!canWrite} {...form.register('amount')} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="currency">Currency</Label>
                  <Input id="currency" readOnly={!canWrite} {...form.register('currency')} />
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="stage_id">Stage</Label>
                <select
                  id="stage_id"
                  disabled={!canWrite}
                  {...form.register('stage_id')}
                  className="h-10 w-full rounded-md border bg-background px-3 text-sm disabled:opacity-50"
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
                <Label htmlFor="expected_close_date">Expected close date</Label>
                <Input
                  id="expected_close_date"
                  type="date"
                  readOnly={!canWrite}
                  {...form.register('expected_close_date')}
                />
              </div>
              {canWrite ? (
                <>
                  <Button
                    type="submit"
                    disabled={updateMutation.isPending || !form.formState.isDirty}
                  >
                    {updateMutation.isPending ? 'Saving...' : 'Save changes'}
                  </Button>
                  {updateMutation.isSuccess && <p className="text-sm text-green-600">Saved</p>}
                </>
              ) : (
                <p className="text-sm text-muted-foreground">
                  You have read-only access. Ask an admin to make changes.
                </p>
              )}
            </form>
          </CardContent>
        </Card>

        <CustomPropertiesEditor
          definitions={definitions}
          values={properties}
          isLoading={propsLoading}
          canWrite={canWrite}
          isSaving={updateProps.isPending}
          saveError={updateProps.isError ? (updateProps.error as Error).message : null}
          onSave={(data) => updateProps.mutate(data)}
        />

        <NotesPanel entityType="deal" entityId={dealId} />
        <TasksPanel scope={{ entity_type: 'deal', entity_id: dealId }} />
      </div>
    </div>
  );
}
