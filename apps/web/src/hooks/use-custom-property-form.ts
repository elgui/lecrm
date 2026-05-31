import * as React from 'react';
import type { PropertyDefinition } from '@/lib/types';

// Form state is keyed by property_key. Inputs are strings (and booleans for
// checkboxes); coerceCustomProperty converts them back to typed values on save.
export type CustomPropertyFormState = Record<string, string | boolean>;

// coerceCustomProperty converts a form value back to the typed value the
// metadata validator expects. Empty strings drop the key entirely so optional
// properties aren't written as "". Booleans are always emitted (false is a
// meaningful value, not "unset").
export function coerceCustomProperty(
  def: PropertyDefinition,
  raw: unknown,
): unknown | undefined {
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

// seedFormState builds the initial editable form values from stored properties,
// one entry per definition so every field is controlled from first render.
function seedFormState(
  definitions: PropertyDefinition[],
  values: Record<string, unknown> | undefined,
): CustomPropertyFormState {
  const next: CustomPropertyFormState = {};
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
  return next;
}

export interface CustomPropertyForm {
  form: CustomPropertyFormState;
  set: (key: string, value: string | boolean) => void;
  isDirty: boolean;
  buildPayload: () => Record<string, unknown>;
}

/**
 * useCustomPropertyForm owns the editable state for a record's custom
 * properties. It re-seeds whenever the definitions or stored values change
 * (e.g. after a save invalidates the query), tracks a dirty flag against that
 * seeded baseline, and builds the typed PUT payload. Lifting this out of the
 * editor component lets a record-detail page combine it with the core-field
 * form behind a single save action.
 */
export function useCustomPropertyForm(
  definitions: PropertyDefinition[] | undefined,
  values: Record<string, unknown> | undefined,
): CustomPropertyForm {
  const [form, setForm] = React.useState<CustomPropertyFormState>({});
  const baselineRef = React.useRef('{}');

  React.useEffect(() => {
    if (!definitions) return;
    const seeded = seedFormState(definitions, values);
    setForm(seeded);
    baselineRef.current = JSON.stringify(seeded);
  }, [definitions, values]);

  const set = React.useCallback((key: string, value: string | boolean) => {
    setForm((f) => ({ ...f, [key]: value }));
  }, []);

  const isDirty = JSON.stringify(form) !== baselineRef.current;

  const buildPayload = React.useCallback(() => {
    const payload: Record<string, unknown> = {};
    if (!definitions) return payload;
    for (const def of definitions) {
      const coerced = coerceCustomProperty(def, form[def.property_key]);
      if (coerced !== undefined) payload[def.property_key] = coerced;
    }
    return payload;
  }, [definitions, form]);

  return { form, set, isDirty, buildPayload };
}
