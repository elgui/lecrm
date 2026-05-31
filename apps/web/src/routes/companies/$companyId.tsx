import { createRoute, Link, useNavigate } from '@tanstack/react-router';
import { useForm } from 'react-hook-form';
import {
  useCompany,
  useUpdateCompany,
  useDeleteCompany,
} from '@/hooks/use-companies';
import { useMe } from '@/hooks/use-me';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Avatar } from '@/components/ui/avatar';
import { ArrowLeft, Trash2 } from 'lucide-react';
import { NotesPanel } from '@/components/notes-panel';
import { TasksPanel } from '@/components/tasks-panel';
import { RecordSaveBar } from '@/components/record-save-bar';
import { Route as rootRoute } from '../__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/companies/$companyId',
  component: CompanyDetail,
});

interface CompanyFormData {
  name: string;
  domain: string;
  industry: string;
  size: string;
}

function CompanyDetail() {
  const { companyId } = Route.useParams();
  const navigate = useNavigate();
  const { data: company, isLoading } = useCompany(companyId);
  const updateMutation = useUpdateCompany(companyId);
  const deleteMutation = useDeleteCompany();
  const { permissions } = useMe();
  const canWrite = permissions.can_write;

  const form = useForm<CompanyFormData>({
    values: company
      ? {
          name: company.name,
          domain: company.domain ?? '',
          industry: company.industry ?? '',
          size: company.size ?? '',
        }
      : undefined,
  });

  const onSubmit = form.handleSubmit((data) => {
    updateMutation.mutate({
      name: data.name,
      domain: data.domain || null,
      industry: data.industry || null,
      size: data.size || null,
    });
  });

  const saveError = updateMutation.isError
    ? (updateMutation.error as Error).message
    : null;

  const onDelete = () => {
    if (!window.confirm('Supprimer cette entreprise ? Cette action est irréversible.')) return;
    deleteMutation.mutate(companyId, {
      onSuccess: () => navigate({ to: '/companies' }),
    });
  };

  if (isLoading) {
    return (
      <div className="space-y-4 p-8">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (!company) {
    return (
      <div className="p-8">
        <p className="text-destructive">Entreprise introuvable</p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-5xl p-8">
      <Link
        to="/companies"
        className="mb-4 inline-flex items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Retour aux entreprises
      </Link>
      <div className="mb-6 flex items-start justify-between gap-4">
        <div className="flex items-center gap-3">
          <Avatar
            name={company.name || '?'}
            seed={company.id}
            size="lg"
            className="rounded-lg"
          />
          <div>
            <h1 className="text-xl font-semibold tracking-tight">
              {company.name}
            </h1>
            {company.domain && (
              <p className="text-sm text-muted-foreground">{company.domain}</p>
            )}
          </div>
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
            <CardTitle className="text-lg">Détails</CardTitle>
          </CardHeader>
          <CardContent>
            <form onSubmit={onSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="name">Nom</Label>
                <Input id="name" readOnly={!canWrite} {...form.register('name', { required: true })} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="domain">Domaine</Label>
                <Input id="domain" readOnly={!canWrite} {...form.register('domain')} />
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="industry">Secteur</Label>
                  <Input id="industry" readOnly={!canWrite} {...form.register('industry')} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="size">Taille</Label>
                  <Input id="size" readOnly={!canWrite} {...form.register('size')} />
                </div>
              </div>
              {/* Submit on Enter; the RecordSaveBar below is the single,
                  consistent save action across every record-detail page. */}
              <button type="submit" className="hidden" aria-hidden tabIndex={-1} />
            </form>
          </CardContent>
        </Card>

        <RecordSaveBar
          className="lg:col-span-2"
          canWrite={canWrite}
          isDirty={form.formState.isDirty}
          isSaving={updateMutation.isPending}
          isSuccess={updateMutation.isSuccess}
          error={saveError}
          onSave={() => void onSubmit()}
        />

        <NotesPanel entityType="company" entityId={companyId} />
        <TasksPanel scope={{ entity_type: 'company', entity_id: companyId }} />
      </div>
    </div>
  );
}
