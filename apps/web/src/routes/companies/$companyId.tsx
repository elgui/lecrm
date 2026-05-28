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
import { ArrowLeft, Trash2 } from 'lucide-react';
import { NotesPanel } from '@/components/notes-panel';
import { TasksPanel } from '@/components/tasks-panel';
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

  const onDelete = () => {
    if (!window.confirm('Delete this company? This cannot be undone.')) return;
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
        <p className="text-destructive">Company not found</p>
      </div>
    );
  }

  return (
    <div className="p-8">
      <div className="mb-6 flex items-start justify-between">
        <div>
          <Link
            to="/companies"
            className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
          >
            <ArrowLeft className="h-4 w-4" />
            Back to companies
          </Link>
          <h1 className="text-2xl font-semibold">{company.name}</h1>
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
            <CardTitle className="text-lg">Details</CardTitle>
          </CardHeader>
          <CardContent>
            <form onSubmit={onSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="name">Name</Label>
                <Input id="name" readOnly={!canWrite} {...form.register('name', { required: true })} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="domain">Domain</Label>
                <Input id="domain" readOnly={!canWrite} {...form.register('domain')} />
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="industry">Industry</Label>
                  <Input id="industry" readOnly={!canWrite} {...form.register('industry')} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="size">Size</Label>
                  <Input id="size" readOnly={!canWrite} {...form.register('size')} />
                </div>
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

        <NotesPanel entityType="company" entityId={companyId} />
        <TasksPanel scope={{ entity_type: 'company', entity_id: companyId }} />
      </div>
    </div>
  );
}
