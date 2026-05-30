import { createRoute } from '@tanstack/react-router';
import { useState } from 'react';
import { useMe } from '@/hooks/use-me';
import {
  useDefinitions,
  useCreateDefinition,
  useDeleteDefinition,
  type DefinitionParentType,
} from '@/hooks/use-metadata-definitions';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card';
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from '@/components/ui/table';
import type { PropertyType } from '@/lib/types';
import { Route as rootRoute } from '../__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/custom-fields',
  component: CustomFieldsPage,
});

const PARENT_TYPES: { value: DefinitionParentType; label: string }[] = [
  { value: 'contact', label: 'Contact' },
  { value: 'deal', label: 'Deal' },
];

const PROPERTY_TYPES: PropertyType[] = [
  'string',
  'number',
  'boolean',
  'enum',
  'date',
  'json',
];

// property_key must be a stable identifier: lowercase letters, digits and
// underscores, starting with a letter. Mirrors the storage convention so the
// key round-trips cleanly through the JSON document and report columns.
const KEY_PATTERN = /^[a-z][a-z0-9_]*$/;

// The API returns errors as `{"error":"..."}`; ApiError carries the raw body
// as its message. Surface the human-readable field when present.
function apiErrorMessage(err: unknown): string {
  const raw = err instanceof Error ? err.message : String(err);
  try {
    const parsed = JSON.parse(raw) as { error?: string };
    if (parsed && typeof parsed.error === 'string') return parsed.error;
  } catch {
    /* not JSON — fall through to the raw text */
  }
  return raw;
}

function CustomFieldsPage() {
  const { permissions, isLoading: meLoading } = useMe();
  const [parentType, setParentType] = useState<DefinitionParentType>('contact');

  if (meLoading) {
    return (
      <div className="space-y-4 p-8">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  // Schema changes are an admin+ capability (can_write === RoleAdmin or above).
  if (!permissions.can_write) {
    return (
      <div className="p-8">
        <h1 className="mb-2 text-2xl font-semibold">Custom Fields</h1>
        <p className="text-destructive">
          Only workspace admins can manage custom fields.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6 p-8">
      <div>
        <h1 className="text-2xl font-semibold">Custom Fields</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Add any field, of any type, to any object — live, no engineer
          required.
        </p>
      </div>

      <div className="flex gap-2">
        {PARENT_TYPES.map((pt) => (
          <Button
            key={pt.value}
            variant={parentType === pt.value ? 'default' : 'outline'}
            size="sm"
            onClick={() => setParentType(pt.value)}
          >
            {pt.label}
          </Button>
        ))}
      </div>

      <CreateFieldForm parentType={parentType} />
      <DefinitionsTable parentType={parentType} />
    </div>
  );
}

function CreateFieldForm({
  parentType,
}: {
  parentType: DefinitionParentType;
}) {
  const create = useCreateDefinition();

  const [propertyKey, setPropertyKey] = useState('');
  const [propertyType, setPropertyType] = useState<PropertyType>('string');
  const [required, setRequired] = useState(false);
  const [allowedValuesRaw, setAllowedValuesRaw] = useState('');
  const [keyError, setKeyError] = useState<string | null>(null);

  const reset = () => {
    setPropertyKey('');
    setPropertyType('string');
    setRequired(false);
    setAllowedValuesRaw('');
    setKeyError(null);
  };

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const key = propertyKey.trim();
    if (!KEY_PATTERN.test(key)) {
      setKeyError(
        'Key must be lowercase letters, digits and underscores, starting with a letter (e.g. account_tier).',
      );
      return;
    }
    setKeyError(null);

    const allowedValues =
      propertyType === 'enum'
        ? allowedValuesRaw
            .split(',')
            .map((v) => v.trim())
            .filter(Boolean)
        : undefined;

    if (propertyType === 'enum' && (!allowedValues || allowedValues.length === 0)) {
      setKeyError('Enum fields need at least one allowed value.');
      return;
    }

    create.mutate(
      {
        parent_type: parentType,
        property_key: key,
        property_type: propertyType,
        required,
        ...(allowedValues ? { allowed_values: allowedValues } : {}),
      },
      { onSuccess: reset },
    );
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">Add a custom field</CardTitle>
        <CardDescription>
          Define a new property for every {parentType}. It appears immediately
          on the record editor.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="flex flex-wrap items-end gap-3">
            <div className="space-y-2">
              <Label htmlFor="cf-key">Field key</Label>
              <Input
                id="cf-key"
                value={propertyKey}
                onChange={(e) => setPropertyKey(e.target.value)}
                placeholder="account_tier"
                className="w-64"
                autoComplete="off"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="cf-type">Type</Label>
              <select
                id="cf-type"
                value={propertyType}
                onChange={(e) =>
                  setPropertyType(e.target.value as PropertyType)
                }
                className="h-9 rounded-md border bg-background px-3 text-sm"
              >
                {PROPERTY_TYPES.map((t) => (
                  <option key={t} value={t}>
                    {t}
                  </option>
                ))}
              </select>
            </div>
            <label className="flex items-center gap-2 pb-2 text-sm">
              <input
                type="checkbox"
                checked={required}
                onChange={(e) => setRequired(e.target.checked)}
                className="h-4 w-4 rounded border"
              />
              Required
            </label>
            <Button
              type="submit"
              disabled={create.isPending || !propertyKey.trim()}
            >
              {create.isPending ? 'Adding…' : 'Add field'}
            </Button>
          </div>

          {propertyType === 'enum' && (
            <div className="space-y-2">
              <Label htmlFor="cf-allowed">Allowed values</Label>
              <Input
                id="cf-allowed"
                value={allowedValuesRaw}
                onChange={(e) => setAllowedValuesRaw(e.target.value)}
                placeholder="bronze, silver, gold"
                className="w-full max-w-lg"
              />
              <p className="text-xs text-muted-foreground">
                Comma-separated list of options.
              </p>
            </div>
          )}

          {keyError && <p className="text-sm text-destructive">{keyError}</p>}
          {create.isError && (
            <p className="text-sm text-destructive">
              {apiErrorMessage(create.error)}
            </p>
          )}
        </form>
      </CardContent>
    </Card>
  );
}

function DefinitionsTable({
  parentType,
}: {
  parentType: DefinitionParentType;
}) {
  const { data: definitions, isLoading, error } = useDefinitions(parentType);
  const remove = useDeleteDefinition();

  const onDelete = (id: string, key: string) => {
    if (
      !window.confirm(
        `Delete the "${key}" field? Existing values on every ${parentType} are removed.`,
      )
    ) {
      return;
    }
    remove.mutate(id);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">
          {parentType === 'contact' ? 'Contact' : 'Deal'} fields
        </CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading && <Skeleton className="h-32 w-full" />}
        {error && (
          <p className="text-destructive">
            Failed to load fields: {apiErrorMessage(error)}
          </p>
        )}
        {definitions && definitions.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No custom fields yet. Add one above.
          </p>
        )}
        {definitions && definitions.length > 0 && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Key</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Required</TableHead>
                <TableHead>Allowed values</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {definitions.map((d) => (
                <TableRow key={d.id}>
                  <TableCell className="font-mono text-sm font-medium">
                    {d.property_key}
                  </TableCell>
                  <TableCell>
                    <Badge variant="secondary">{d.property_type}</Badge>
                  </TableCell>
                  <TableCell>
                    {d.required ? (
                      <Badge>required</Badge>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {d.allowed_values && d.allowed_values.length > 0
                      ? d.allowed_values.join(', ')
                      : '—'}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={remove.isPending}
                      onClick={() => onDelete(d.id, d.property_key)}
                    >
                      Delete
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
