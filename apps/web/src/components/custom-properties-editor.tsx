import type { PropertyDefinition } from '@/lib/types';
import type { CustomPropertyFormState } from '@/hooks/use-custom-property-form';
import { customFieldLabel } from '@/lib/format-property';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';

interface CustomPropertiesFieldsProps {
  definitions: PropertyDefinition[] | undefined;
  form: CustomPropertyFormState;
  onChange: (key: string, value: string | boolean) => void;
  isLoading: boolean;
  canWrite: boolean;
}

// CustomPropertiesFields renders one input per definition (string, number,
// boolean, enum, date, json) using the workspace's metadata definitions. It is
// a controlled, button-less section: the parent record-detail page owns the
// form state (useCustomPropertyForm) and a single save action shared with the
// core fields, so editing a custom property and a core field then saving once
// persists both — no more two-button data-loss trap. Field labels come from
// customFieldLabel (admin display_name, else a prettified key) rather than the
// raw snake_case key.
export function CustomPropertiesFields({
  definitions,
  form,
  onChange,
  isLoading,
  canWrite,
}: CustomPropertiesFieldsProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">Informations complémentaires</CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading && <Skeleton className="h-32 w-full" />}

        {!isLoading && (!definitions || definitions.length === 0) && (
          <p className="text-sm text-muted-foreground">
            Aucun champ personnalisé défini pour cet espace de travail.
            Ajoutez-en dans Réglages → Champs personnalisés.
          </p>
        )}

        {!isLoading && definitions && definitions.length > 0 && (
          <div className="space-y-4">
            {definitions.map((def) => {
              const id = `cf-${def.property_key}`;
              const val = form[def.property_key];
              return (
                <div key={def.id} className="space-y-2">
                  <Label htmlFor={id}>
                    {customFieldLabel(def)}
                    {def.required && <span className="ml-1 text-destructive">*</span>}
                  </Label>
                  {def.property_type === 'boolean' ? (
                    <input
                      id={id}
                      type="checkbox"
                      checked={val === true}
                      disabled={!canWrite}
                      onChange={(e) => onChange(def.property_key, e.target.checked)}
                      className="ml-1 h-4 w-4 cursor-pointer rounded align-middle accent-primary"
                    />
                  ) : def.property_type === 'enum' ? (
                    <select
                      id={id}
                      value={String(val ?? '')}
                      disabled={!canWrite}
                      onChange={(e) => onChange(def.property_key, e.target.value)}
                      className="h-10 w-full rounded-md border border-input bg-card px-3 text-sm shadow-xs focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/25 disabled:opacity-50"
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
                      onChange={(e) => onChange(def.property_key, e.target.value)}
                    />
                  )}
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
