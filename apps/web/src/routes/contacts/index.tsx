import * as React from 'react';
import { createRoute, Link, useNavigate } from '@tanstack/react-router';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Plus, Users, Search, ChevronRight, GitMerge } from 'lucide-react';
import { useContacts, useCreateContact, useContactDefinitions } from '@/hooks/use-contacts';
import { useCompanyMap } from '@/hooks/use-companies';
import { useBatchProperties } from '@/hooks/use-metadata-definitions';
import { useMe } from '@/hooks/use-me';
import { formatPropertyValue, customFieldLabel } from '@/lib/format-property';
import { formatDate } from '@/lib/format';
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
import { DedupWizard } from '@/components/dedup-wizard';
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
  path: '/contacts',
  // `?new=true` (e.g. from the mobile create FAB) auto-opens the create form.
  validateSearch: (search: Record<string, unknown>): { new?: boolean } => ({
    new: search.new === true || search.new === 'true' ? true : undefined,
  }),
  component: ContactList,
});

const contactSchema = z.object({
  first_name: z.string().min(1, 'Le prénom est requis'),
  last_name: z.string().min(1, 'Le nom est requis'),
  email: z.string().email('E-mail invalide').or(z.literal('')),
  phone: z.string(),
});
type ContactForm = z.infer<typeof contactSchema>;

function CreateContactForm({ onDone }: { onDone: () => void }) {
  const create = useCreateContact();
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<ContactForm>({
    resolver: zodResolver(contactSchema),
    defaultValues: { first_name: '', last_name: '', email: '', phone: '' },
  });

  const onSubmit = handleSubmit((data) => {
    create.mutate(
      {
        first_name: data.first_name,
        last_name: data.last_name,
        email: data.email || null,
        phone: data.phone || null,
        company_id: null,
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
              <Label htmlFor="first_name">Prénom</Label>
              <Input id="first_name" {...register('first_name')} />
              {errors.first_name && (
                <p className="text-sm text-destructive">{errors.first_name.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="last_name">Nom</Label>
              <Input id="last_name" {...register('last_name')} />
              {errors.last_name && (
                <p className="text-sm text-destructive">{errors.last_name.message}</p>
              )}
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="email">E-mail</Label>
              <Input id="email" type="email" {...register('email')} />
              {errors.email && (
                <p className="text-sm text-destructive">{errors.email.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="phone">Téléphone</Label>
              <Input id="phone" {...register('phone')} />
            </div>
          </div>
          <div className="flex gap-2">
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? 'Création…' : 'Créer le contact'}
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

function contactDisplayName(contact: {
  first_name?: string | null;
  last_name?: string | null;
  email?: string | null;
}): string {
  return (
    `${contact.first_name ?? ''} ${contact.last_name ?? ''}`.trim() ||
    contact.email ||
    'Inconnu'
  );
}

function ContactList() {
  const { data, isLoading, error } = useContacts();
  const { permissions } = useMe();
  const companyMap = useCompanyMap();
  const { new: openCreate } = Route.useSearch();
  const navigate = useNavigate();
  const [creating, setCreating] = React.useState(false);
  const [importing, setImporting] = React.useState(false);
  const [deduping, setDeduping] = React.useState(false);
  const [query, setQuery] = React.useState('');

  // Honour `?new=true` (mobile create FAB) once, then strip the param so a
  // refresh or back-nav doesn't keep re-opening the form.
  React.useEffect(() => {
    if (openCreate) {
      setCreating(true);
      navigate({ to: '/contacts', search: {}, replace: true });
    }
  }, [openCreate, navigate]);

  // First couple of custom fields shown as columns; values batch-fetched for
  // the whole page in one request (no N+1).
  const { data: defs } = useContactDefinitions();
  const customCols = (defs ?? []).slice(0, 2);
  // Only fetch values when there's at least one custom column to render.
  const contactIds = customCols.length > 0 ? (data?.data.map((c) => c.id) ?? []) : [];
  const { data: propsById } = useBatchProperties('contact', contactIds);

  const colSpan = 4 + customCols.length;

  const q = query.trim().toLowerCase();
  const rows = (data?.data ?? []).filter((c) => {
    if (!q) return true;
    const company = c.company_id ? (companyMap.get(c.company_id) ?? '') : '';
    return `${contactDisplayName(c)} ${c.email ?? ''} ${company}`
      .toLowerCase()
      .includes(q);
  });

  return (
    <div className="mx-auto max-w-7xl p-4 md:p-8">
      <PageHeader
        title="Contacts"
        description="Gérez vos contacts et vos relations"
        actions={
          <>
            <ExportButton resource="contacts" />
            {permissions.can_write && (
              <Button variant="outline" size="sm" onClick={() => setImporting(true)}>
                <Plus className="mr-1 h-4 w-4" />
                Importer CSV
              </Button>
            )}
            {permissions.can_write && (
              <Button variant="outline" size="sm" onClick={() => setDeduping(true)}>
                <GitMerge className="mr-1 h-4 w-4" />
                Doublons
              </Button>
            )}
            {permissions.can_write && !creating && (
              <Button onClick={() => setCreating(true)}>
                <Plus />
                Nouveau contact
              </Button>
            )}
          </>
        }
      />

      {importing && (
        <CsvImportWizard entity="contacts" onClose={() => setImporting(false)} />
      )}

      {deduping && (
        <DedupWizard entity="contacts" onClose={() => setDeduping(false)} />
      )}

      {creating && <CreateContactForm onDone={() => setCreating(false)} />}

      {error && (
        <p className="text-destructive">Échec du chargement des contacts : {error.message}</p>
      )}

      {/* Pinned search — sticks to the top on mobile so it stays reachable
          while the list scrolls under it. */}
      {!isLoading && data && data.data.length > 0 && (
        <div className="sticky top-0 z-10 -mx-4 mb-3 bg-background/95 px-4 py-2 backdrop-blur md:static md:mx-0 md:mb-4 md:bg-transparent md:px-0 md:py-0 md:backdrop-blur-none">
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Rechercher un contact…"
              aria-label="Rechercher un contact"
              className="pl-9"
            />
          </div>
        </div>
      )}

      {/* Mobile: full-width two-line rows (avatar · name + company · chevron).
          No table columns — denser and touch-friendly at 390px. */}
      <div className="md:hidden">
        {!isLoading && data && data.data.length > 0 && (
          <Card className="divide-y divide-border overflow-hidden">
            {rows.length === 0 ? (
              <p className="px-4 py-8 text-center text-sm text-muted-foreground">
                Aucun contact ne correspond à « {query} ».
              </p>
            ) : (
              rows.map((contact) => {
                const name = contactDisplayName(contact);
                const companyName = contact.company_id
                  ? companyMap.get(contact.company_id)
                  : undefined;
                return (
                  <Link
                    key={contact.id}
                    to="/contacts/$contactId"
                    params={{ contactId: contact.id }}
                    className="flex items-center gap-3 px-4 py-3 transition-colors active:bg-accent"
                  >
                    <Avatar name={name} seed={contact.id} />
                    <div className="min-w-0 flex-1">
                      <p className="truncate font-medium text-foreground">{name}</p>
                      <p className="truncate text-sm text-muted-foreground">
                        {companyName ?? contact.email ?? '—'}
                      </p>
                    </div>
                    <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                  </Link>
                );
              })
            )}
          </Card>
        )}
        {isLoading && (
          <Card className="divide-y divide-border overflow-hidden">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="flex items-center gap-3 px-4 py-3">
                <Skeleton className="h-9 w-9 rounded-full" />
                <div className="flex-1 space-y-1.5">
                  <Skeleton className="h-4 w-32" />
                  <Skeleton className="h-3 w-24" />
                </div>
              </div>
            ))}
          </Card>
        )}
        {!isLoading && (!data || data.data.length === 0) && !creating && (
          <Card>
            <EmptyState
              icon={Users}
              title="Aucun contact"
              description="Ajoutez votre premier contact pour commencer à construire vos relations."
              action={
                permissions.can_write && (
                  <Button onClick={() => setCreating(true)}>
                    <Plus />
                    Nouveau contact
                  </Button>
                )
              }
            />
          </Card>
        )}
      </div>

      {/* Desktop: the full data table. */}
      <Card className="hidden overflow-hidden md:block">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>Nom</TableHead>
              <TableHead>E-mail</TableHead>
              <TableHead>Entreprise</TableHead>
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
                    <div className="flex items-center gap-3">
                      <Skeleton className="h-9 w-9 rounded-full" />
                      <Skeleton className="h-4 w-32" />
                    </div>
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-40" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-24" />
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
                      icon={Users}
                      title="Aucun contact"
                      description="Ajoutez votre premier contact pour commencer à construire vos relations."
                      action={
                        permissions.can_write && (
                          <Button onClick={() => setCreating(true)}>
                            <Plus />
                            Nouveau contact
                          </Button>
                        )
                      }
                    />
                  </TableCell>
                </TableRow>
              )
            ) : rows.length === 0 ? (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={colSpan} className="py-8 text-center text-muted-foreground">
                  Aucun contact ne correspond à « {query} ».
                </TableCell>
              </TableRow>
            ) : (
              rows.map((contact) => {
                const name =
                  `${contact.first_name ?? ''} ${contact.last_name ?? ''}`.trim() ||
                  contact.email ||
                  'Inconnu';
                return (
                  <TableRow key={contact.id} className="group">
                    <TableCell>
                      <Link
                        to="/contacts/$contactId"
                        params={{ contactId: contact.id }}
                        className="flex items-center gap-3"
                      >
                        <Avatar name={name} seed={contact.id} />
                        <span className="font-medium text-primary group-hover:underline">
                          {contact.first_name} {contact.last_name}
                        </span>
                      </Link>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {contact.email ?? '—'}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {(() => {
                        const companyName = contact.company_id
                          ? companyMap.get(contact.company_id)
                          : undefined;
                        if (contact.company_id && companyName) {
                          return (
                            <Link
                              to="/companies/$companyId"
                              params={{ companyId: contact.company_id }}
                              className="hover:text-foreground hover:underline"
                            >
                              {companyName}
                            </Link>
                          );
                        }
                        return '—';
                      })()}
                    </TableCell>
                    {customCols.map((def) => {
                      const formatted = formatPropertyValue(
                        def,
                        propsById?.[contact.id]?.[def.property_key],
                      );
                      return (
                        <TableCell key={def.id} className="text-muted-foreground">
                          {formatted || '—'}
                        </TableCell>
                      );
                    })}
                    <TableCell className="text-muted-foreground tabular-nums">
                      {formatDate(contact.created_at)}
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
              {rows.length}{' '}
              {rows.length === 1 ? 'contact' : 'contacts'}
              {q && ` sur ${data.data.length}`}
            </span>
            {data.has_more && (
              <Button variant="ghost" size="sm" disabled>
                Charger plus
              </Button>
            )}
          </div>
        )}
      </Card>
    </div>
  );
}
