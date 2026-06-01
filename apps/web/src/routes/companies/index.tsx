import * as React from 'react';
import { createRoute, Link } from '@tanstack/react-router';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Plus, Building2 } from 'lucide-react';
import { useCompanies, useCreateCompany } from '@/hooks/use-companies';
import { useMe } from '@/hooks/use-me';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent } from '@/components/ui/card';
import { Avatar } from '@/components/ui/avatar';
import { PageHeader } from '@/components/page-header';
import { EmptyState } from '@/components/empty-state';
import { ExportButton } from '@/components/export-button';
import { CsvImportWizard } from '@/components/csv-import-wizard';
import { formatDate } from '@/lib/format';
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
  name: z.string().min(1, 'Le nom est requis'),
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
              <Label htmlFor="name">Nom</Label>
              <Input id="name" {...register('name')} />
              {errors.name && (
                <p className="text-sm text-destructive">{errors.name.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="domain">Domaine</Label>
              <Input id="domain" placeholder="exemple.com" {...register('domain')} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="industry">Secteur</Label>
              <Input id="industry" {...register('industry')} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="size">Taille</Label>
              <Input id="size" placeholder="1-10, 11-50, …" {...register('size')} />
            </div>
          </div>
          <div className="flex gap-2">
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? 'Création…' : 'Créer l’entreprise'}
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

function CompanyList() {
  const { data, isLoading, error } = useCompanies();
  const { permissions } = useMe();
  const [creating, setCreating] = React.useState(false);
  const [importing, setImporting] = React.useState(false);

  return (
    <div className="mx-auto max-w-7xl p-8">
      <PageHeader
        title="Entreprises"
        description="Suivez vos organisations et vos comptes"
        actions={
          <>
            <ExportButton resource="companies" />
            {permissions.can_write && (
              <Button variant="outline" size="sm" onClick={() => setImporting(true)}>
                <Plus className="mr-1 h-4 w-4" />
                Importer CSV
              </Button>
            )}
            {permissions.can_write && !creating && (
              <Button onClick={() => setCreating(true)}>
                <Plus />
                Nouvelle entreprise
              </Button>
            )}
          </>
        }
      />

      {importing && (
        <CsvImportWizard entity="companies" onClose={() => setImporting(false)} />
      )}

      {creating && <CreateCompanyForm onDone={() => setCreating(false)} />}

      {error && (
        <p className="text-destructive">Échec du chargement des entreprises : {error.message}</p>
      )}

      <Card className="overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>Nom</TableHead>
              <TableHead>Domaine</TableHead>
              <TableHead>Créé le</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              Array.from({ length: 6 }).map((_, i) => (
                <TableRow key={i} className="hover:bg-transparent">
                  <TableCell>
                    <div className="flex items-center gap-3">
                      <Skeleton className="h-9 w-9 rounded-md" />
                      <Skeleton className="h-4 w-32" />
                    </div>
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-40" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-20" />
                  </TableCell>
                </TableRow>
              ))
            ) : !data || data.data.length === 0 ? (
              !creating && (
                <TableRow className="hover:bg-transparent">
                  <TableCell colSpan={3} className="p-0">
                    <EmptyState
                      icon={Building2}
                      title="Aucune entreprise"
                      description="Ajoutez votre première entreprise pour suivre vos comptes et organisations."
                      action={
                        permissions.can_write && (
                          <Button onClick={() => setCreating(true)}>
                            <Plus />
                            Nouvelle entreprise
                          </Button>
                        )
                      }
                    />
                  </TableCell>
                </TableRow>
              )
            ) : (
              data.data.map((company) => (
                <TableRow key={company.id} className="group">
                  <TableCell>
                    <Link
                      to="/companies/$companyId"
                      params={{ companyId: company.id }}
                      className="flex items-center gap-3"
                    >
                      <Avatar
                        name={company.name || '?'}
                        seed={company.id}
                        className="rounded-md"
                      />
                      <span className="font-medium text-primary group-hover:underline">
                        {company.name}
                      </span>
                    </Link>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {company.domain ?? '—'}
                  </TableCell>
                  <TableCell className="text-muted-foreground tabular-nums">
                    {formatDate(company.created_at)}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        {!isLoading && data && data.data.length > 0 && (
          <div className="flex items-center justify-between border-t border-border px-4 py-2.5 text-xs text-muted-foreground">
            <span>
              {data.data.length}{' '}
              {data.data.length === 1 ? 'entreprise' : 'entreprises'}
            </span>
          </div>
        )}
      </Card>
    </div>
  );
}
