import * as React from 'react';
import { createRoute, Link } from '@tanstack/react-router';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Plus } from 'lucide-react';
import { useContacts, useCreateContact } from '@/hooks/use-contacts';
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
  const [creating, setCreating] = React.useState(false);

  return (
    <div className="p-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Contacts</h1>
        <div className="flex items-center gap-2">
          <ExportButton resource="contacts" />
          {permissions.can_write && !creating && (
            <Button size="sm" onClick={() => setCreating(true)}>
              <Plus className="mr-2 h-4 w-4" />
              New contact
            </Button>
          )}
        </div>
      </div>

      {creating && <CreateContactForm onDone={() => setCreating(false)} />}

      {isLoading && (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      )}

      {error && (
        <p className="text-destructive">Failed to load contacts: {error.message}</p>
      )}

      {data && data.data.length === 0 && !creating && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <p className="text-lg text-muted-foreground">No contacts yet</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Create your first contact to get started.
          </p>
        </div>
      )}

      {data && data.data.length > 0 && (
        <>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Email</TableHead>
                <TableHead>Company</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.data.map((contact) => (
                <TableRow key={contact.id}>
                  <TableCell>
                    <Link
                      to="/contacts/$contactId"
                      params={{ contactId: contact.id }}
                      className="font-medium text-primary hover:underline"
                    >
                      {contact.first_name} {contact.last_name}
                    </Link>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {contact.email ?? '-'}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {contact.company_id ? contact.company_id.slice(0, 8) + '…' : '-'}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {new Date(contact.created_at).toLocaleDateString()}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>

          {data.has_more && (
            <div className="mt-4 flex justify-center">
              <Button variant="outline" disabled>
                Load more
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
