import * as React from 'react';
import type { PropertyDefinition } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';

interface CustomPropertiesEditorProps {
  definitions: PropertyDefinition[] | undefined;
  values: Record<string, unknown> | undefined;
  isLoading: boolean;
  canWrite: boolean;
  onSave: (data: Record<string, unknown>) => void;
  isSaving: boolean;
  saveError?: string | null;
}

// coerce converts a form string back to the typed value the metadata
// validator expects for a given definition. Empty strings drop the key
// entirely so optional properties aren't written as "".
function coerce(def: PropertyDefinition, raw: unknown): unknown | undefined {
  if (def.property_type === 'boolean') return raw === true || raw === 'true';
  if (raw === '' || raw === undefined || raw === null) return undefined;
  switch (def.property_type) {
    case 'number':
      return typeof raw === 'number' ? raw : Number(raw);
    case 'json':
      try {
        return JSON.parse(String(raw));
      } catch {
        return String(raw);
      }
    default:
      return String(raw);
  }
}

// CustomPropertiesEditor renders one field per definition (string, number,
// boolean, enum, date, json) and PUTs the whole record on save. Definitions
// come from the workspace's metadata engine; without any, it explains how
// they're provisioned rather than showing an empty form.
export function CustomPropertiesEditor({
  definitions,
  values,
  isLoading,
  canWrite,
  onSave,
  isSaving,
  saveError,
}: CustomPropertiesEditorProps) {
  const [form, setForm] = React.useState<Record<string, string | boolean>>({});

  // Seed form state when values or definitions arrive.
  React.useEffect(() => {
    if (!definitions) return;
    const next: Record<string, string | boolean> = {};
    for (const def of definitions) {
      const v = values?.[def.property_key];
      if (def.property_type === 'boolean') {
        next[def.property_key] = v === true;
      } else if (def.property_type === 'json') {
        next[def.property_key] = v === undefined ? '' : JSON.stringify(v);
      } else {
        next[def.property_key] = v === undefined || v === null ? '' : String(v);
      }
    }
    setForm(next);
  }, [definitions, values]);

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!definitions) return;
    const payload: Record<string, unknown> = {};
    for (const def of definitions) {
      const c = coerce(def, form[def.property_key]);
      if (c !== undefined) payload[def.property_key] = c;
    }
    onSave(payload);
  };

  const set = (key: string, value: string | boolean) =>
    setForm((f) => ({ ...f, [key]: value }));

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">Custom Properties</CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading && <Skeleton className="h-32 w-full" />}

        {!isLoading && (!definitions || definitions.length === 0) && (
          <p className="text-sm text-muted-foreground">
            No custom properties defined for this workspace. Define them via
            the metadata API (or the integrator methodology config).
          </p>
        )}

        {!isLoading && definitions && definitions.length > 0 && (
          <form onSubmit={onSubmit} className="space-y-4">
            {definitions.map((def) => {
              const id = `cf-${def.property_key}`;
              const val = form[def.property_key];
              return (
                <div key={def.id} className="space-y-2">
                  <Label htmlFor={id}>
                    {def.property_key}
                    {def.required && <span className="ml-1 text-destructive">*</span>}
                  </Label>
                  {def.property_type === 'boolean' ? (
                    <input
                      id={id}
                      type="checkbox"
                      checked={val === true}
                      disabled={!canWrite}
                      onChange={(e) => set(def.property_key, e.target.checked)}
                      className="ml-1 h-4 w-4 align-middle"
                    />
                  ) : def.property_type === 'enum' ? (
                    <select
                      id={id}
                      value={String(val ?? '')}
                      disabled={!canWrite}
                      onChange={(e) => set(def.property_key, e.target.value)}
                      className="h-10 w-full rounded-md border bg-background px-3 text-sm disabled:opacity-50"
                    >
                      <option value="">—</option>
                      {(def.allowed_values ?? []).map((opt) => (
                        <option key={opt} value={opt}>
                          {opt}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <Input
                      id={id}
                      type={
                        def.property_type === 'number'
                          ? 'number'
                          : def.property_type === 'date'
                            ? 'date'
                            : 'text'
                      }
                      value={String(val ?? '')}
                      readOnly={!canWrite}
                      onChange={(e) => set(def.property_key, e.target.value)}
                    />
                  )}
                </div>
              );
            })}
            {canWrite ? (
              <div className="space-y-1">
                <Button type="submit" disabled={isSaving}>
                  {isSaving ? 'Saving…' : 'Save properties'}
                </Button>
                {saveError && <p className="text-sm text-destructive">{saveError}</p>}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                You have read-only access to custom properties.
              </p>
            )}
          </form>
        )}
      </CardContent>
    </Card>
  );
}
