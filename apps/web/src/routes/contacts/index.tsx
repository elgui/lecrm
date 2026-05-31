import * as React from 'react';
import { createRoute, Link } from '@tanstack/react-router';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Plus, Users } from 'lucide-react';
import { useContacts, useCreateContact, useContactDefinitions } from '@/hooks/use-contacts';
import { useCompanyMap } from '@/hooks/use-companies';
import { useBatchProperties } from '@/hooks/use-metadata-definitions';
import { useMe } from '@/hooks/use-me';
import { formatPropertyValue } from '@/lib/format-property';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent } from '@/components/ui/card';
import { Avatar } from '@/components/ui/avatar';
import { PageHeader } from '@/components/page-header';
import { EmptyState } from '@/components/empty-state';
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
  path: '/contacts',
  component: ContactList,
});

const contactSchema = z.object({
  first_name: z.string().min(1, 'First name is required'),
  last_name: z.string().min(1, 'Last name is required'),
  email: z.string().email('Invalid email').or(z.literal('')),
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
              <Label htmlFor="first_name">First name</Label>
              <Input id="first_name" {...register('first_name')} />
              {errors.first_name && (
                <p className="text-sm text-destructive">{errors.first_name.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="last_name">Last name</Label>
              <Input id="last_name" {...register('last_name')} />
              {errors.last_name && (
                <p className="text-sm text-destructive">{errors.last_name.message}</p>
              )}
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input id="email" type="email" {...register('email')} />
              {errors.email && (
                <p className="text-sm text-destructive">{errors.email.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="phone">Phone</Label>
              <Input id="phone" {...register('phone')} />
            </div>
          </div>
          <div className="flex gap-2">
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? 'Creating…' : 'Create contact'}
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

function ContactList() {
  const { data, isLoading, error } = useContacts();
  const { permissions } = useMe();
  const companyMap = useCompanyMap();
  const [creating, setCreating] = React.useState(false);

  // First couple of custom fields shown as columns; values batch-fetched for
  // the whole page in one request (no N+1).
  const { data: defs } = useContactDefinitions();
  const customCols = (defs ?? []).slice(0, 2);
  // Only fetch values when there's at least one custom column to render.
  const contactIds = customCols.length > 0 ? (data?.data.map((c) => c.id) ?? []) : [];
  const { data: propsById } = useBatchProperties('contact', contactIds);

  const colSpan = 4 + customCols.length;

  return (
    <div className="mx-auto max-w-7xl p-8">
      <PageHeader
        title="Contacts"
        description="Manage your contacts and relationships"
        actions={
          <>
            <ExportButton resource="contacts" />
            {permissions.can_write && !creating && (
              <Button onClick={() => setCreating(true)}>
                <Plus />
                New contact
              </Button>
            )}
          </>
        }
      />

      {creating && <CreateContactForm onDone={() => setCreating(false)} />}

      {error && (
        <p className="text-destructive">Failed to load contacts: {error.message}</p>
      )}

      <Card className="overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>Name</TableHead>
              <TableHead>Email</TableHead>
              <TableHead>Company</TableHead>
              {customCols.map((def) => (
                <TableHead key={def.id}>{def.property_key}</TableHead>
              ))}
              <TableHead>Created</TableHead>
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
                      title="No contacts yet"
                      description="Add your first contact to start building relationships."
                      action={
                        permissions.can_write && (
                          <Button onClick={() => setCreating(true)}>
                            <Plus />
                            New contact
                          </Button>
                        )
                      }
                    />
                  </TableCell>
                </TableRow>
              )
            ) : (
              data.data.map((contact) => {
                const name =
                  `${contact.first_name ?? ''} ${contact.last_name ?? ''}`.trim() ||
                  contact.email ||
                  'Unknown';
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
                      {new Date(contact.created_at).toLocaleDateString()}
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
              {data.data.length}{' '}
              {data.data.length === 1 ? 'contact' : 'contacts'}
            </span>
            {data.has_more && (
              <Button variant="ghost" size="sm" disabled>
                Load more
              </Button>
            )}
          </div>
        )}
      </Card>
    </div>
  );
}
