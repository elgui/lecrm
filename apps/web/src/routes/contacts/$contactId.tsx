import { createRoute, Link, useNavigate } from '@tanstack/react-router';
import { useForm } from 'react-hook-form';
import {
  useContact,
  useUpdateContact,
  useDeleteContact,
  useContactProperties,
  useUpdateContactProperties,
  useContactDefinitions,
} from '@/hooks/use-contacts';
import { useCompany } from '@/hooks/use-companies';
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
import { CustomPropertiesFields } from '@/components/custom-properties-editor';
import { RecordSaveBar } from '@/components/record-save-bar';
import { useCustomPropertyForm } from '@/hooks/use-custom-property-form';
import { Route as rootRoute } from '../__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/contacts/$contactId',
  component: ContactDetail,
});

interface ContactFormData {
  first_name: string;
  last_name: string;
  email: string;
  phone: string;
}

function ContactDetail() {
  const { contactId } = Route.useParams();
  const navigate = useNavigate();
  const { data: contact, isLoading } = useContact(contactId);
  const { data: company } = useCompany(contact?.company_id ?? '');
  const { data: properties, isLoading: propsLoading } = useContactProperties(contactId);
  const { data: definitions } = useContactDefinitions();
  const updateMutation = useUpdateContact(contactId);
  const updateProps = useUpdateContactProperties(contactId);
  const deleteMutation = useDeleteContact();
  const { permissions } = useMe();
  const canWrite = permissions.can_write;

  const form = useForm<ContactFormData>({
    values: contact
      ? {
          first_name: contact.first_name,
          last_name: contact.last_name,
          email: contact.email ?? '',
          phone: contact.phone ?? '',
        }
      : undefined,
  });

  const customProps = useCustomPropertyForm(definitions, properties);

  // Single save: persist core fields and custom properties together. Core
  // validation runs first; custom properties only save if the core form is
  // valid (or untouched), so an invalid required field never lets a partial
  // write through. Each mutation only fires when its section is dirty.
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
          first_name: data.first_name,
          last_name: data.last_name,
          email: data.email || null,
          phone: data.phone || null,
        });
      })();
    }
    if (coreOk && customProps.isDirty) {
      updateProps.mutate(customProps.buildPayload());
    }
  };

  const onDelete = () => {
    if (!window.confirm('Delete this contact? This cannot be undone.')) return;
    deleteMutation.mutate(contactId, {
      onSuccess: () => navigate({ to: '/contacts' }),
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

  if (!contact) {
    return (
      <div className="p-8">
        <p className="text-destructive">Contact not found</p>
      </div>
    );
  }

  const fullName = `${contact.first_name ?? ''} ${contact.last_name ?? ''}`.trim();

  return (
    <div className="mx-auto max-w-5xl p-8">
      <Link
        to="/contacts"
        className="mb-4 inline-flex items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to contacts
      </Link>
      <div className="mb-6 flex items-start justify-between gap-4">
        <div className="flex items-center gap-3">
          <Avatar name={fullName || '?'} seed={contact.id} size="lg" />
          <div>
            <h1 className="text-xl font-semibold tracking-tight">
              {contact.first_name} {contact.last_name}
            </h1>
            {contact.email && (
              <p className="text-sm text-muted-foreground">{contact.email}</p>
            )}
          </div>
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
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void onSaveAll();
              }}
              className="space-y-4"
            >
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="first_name">First name</Label>
                  <Input
                    id="first_name"
                    readOnly={!canWrite}
                    {...form.register('first_name', { required: true })}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="last_name">Last name</Label>
                  <Input
                    id="last_name"
                    readOnly={!canWrite}
                    {...form.register('last_name', { required: true })}
                  />
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="email">Email</Label>
                <Input id="email" type="email" readOnly={!canWrite} {...form.register('email')} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="phone">Phone</Label>
                <Input id="phone" readOnly={!canWrite} {...form.register('phone')} />
              </div>
              <div className="space-y-2">
                <Label>Company</Label>
                {contact.company_id && company ? (
                  <p className="text-sm">
                    <Link
                      to="/companies/$companyId"
                      params={{ companyId: company.id }}
                      className="font-medium text-primary hover:underline"
                    >
                      {company.name}
                    </Link>
                  </p>
                ) : (
                  <p className="text-sm text-muted-foreground">—</p>
                )}
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

        <NotesPanel entityType="contact" entityId={contactId} />
        <TasksPanel scope={{ entity_type: 'contact', entity_id: contactId }} />
      </div>
    </div>
  );
}
