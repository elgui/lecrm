import * as React from 'react';
import { createRoute, Link } from '@tanstack/react-router';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Plus } from 'lucide-react';
import { useCompanies, useCreateCompany } from '@/hooks/use-companies';
import { useMe } from '@/hooks/use-me';
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
  path: '/companies',
  component: CompanyList,
});

const companySchema = z.object({
  name: z.string().min(1, 'Name is required'),
  domain: z.string(),
  industry: z.string(),
  size: z.string(),
});
type CompanyForm = z.infer<typeof companySchema>;

function CreateCompanyForm({ onDone }: { onDone: () => void }) {
  const create = useCreateCompany();
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<CompanyForm>({
    resolver: zodResolver(companySchema),
    defaultValues: { name: '', domain: '', industry: '', size: '' },
  });

  const onSubmit = handleSubmit((data) => {
    create.mutate(
      {
        name: data.name,
        domain: data.domain || null,
        industry: data.industry || null,
        size: data.size || null,
      },
      { onSuccess: onDone },
    );
  });

  return (
    <Card className="mb-6">
      <CardContent className="pt-6">
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="name">Name</Label>
              <Input id="name" {...register('name')} />
              {errors.name && (
                <p className="text-sm text-destructive">{errors.name.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="domain">Domain</Label>
              <Input id="domain" placeholder="example.com" {...register('domain')} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="industry">Industry</Label>
              <Input id="industry" {...register('industry')} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="size">Size</Label>
              <Input id="size" placeholder="1-10, 11-50, …" {...register('size')} />
            </div>
          </div>
          <div className="flex gap-2">
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? 'Creating…' : 'Create company'}
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

function CompanyList() {
  const { data, isLoading, error } = useCompanies();
  const { permissions } = useMe();
  const [creating, setCreating] = React.useState(false);

  return (
    <div className="p-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Companies</h1>
        <div className="flex items-center gap-2">
          <ExportButton resource="companies" />
          {permissions.can_write && !creating && (
            <Button size="sm" onClick={() => setCreating(true)}>
              <Plus className="mr-2 h-4 w-4" />
              New company
            </Button>
          )}
        </div>
      </div>

      {creating && <CreateCompanyForm onDone={() => setCreating(false)} />}

      {isLoading && (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      )}

      {error && (
        <p className="text-destructive">Failed to load companies: {error.message}</p>
      )}

      {data && data.data.length === 0 && !creating && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <p className="text-lg text-muted-foreground">No companies yet</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Create your first company to get started.
          </p>
        </div>
      )}

      {data && data.data.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Domain</TableHead>
              <TableHead>Created</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.data.map((company) => (
              <TableRow key={company.id}>
                <TableCell className="font-medium">
                  <Link
                    to="/companies/$companyId"
                    params={{ companyId: company.id }}
                    className="text-primary hover:underline"
                  >
                    {company.name}
                  </Link>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {company.domain ?? '-'}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {new Date(company.created_at).toLocaleDateString()}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
