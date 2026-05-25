import { createRoute, Link } from '@tanstack/react-router';
import { useContacts } from '@/hooks/use-contacts';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
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

function ContactList() {
  const { data, isLoading, error } = useContacts();

  return (
    <div className="p-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Contacts</h1>
      </div>

      {isLoading && (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      )}

      {error && (
        <p className="text-destructive">
          Failed to load contacts: {error.message}
        </p>
      )}

      {data && data.data.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <p className="text-lg text-muted-foreground">No contacts yet</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Contacts will appear here once created via the API.
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
                    {contact.company_name ?? '-'}
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
