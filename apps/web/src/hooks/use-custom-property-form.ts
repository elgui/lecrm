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
 * properties. It seeds from the stored values on load and re-adopts server
 * state only while the form is clean — a background refetch or query
 * invalidation mid-edit will NOT clobber an in-progress draft. After a save
 * settles the values back to match the draft, the baseline is adopted so the
 * dirty flag clears. It tracks that dirty flag and builds the typed PUT
 * payload. Lifting this out of the editor component lets a record-detail page
 * combine it with the core-field form behind a single save action.
 */
export function useCustomPropertyForm(
  definitions: PropertyDefinition[] | undefined,
  values: Record<string, unknown> | undefined,
): CustomPropertyForm {
  const [form, setForm] = React.useState<CustomPropertyFormState>({});
  const baselineRef = React.useRef('{}');
  // Mirror the live form in a ref so the (definitions/values) reseed effect can
  // inspect the current draft without listing `form` as a dependency (which
  // would re-run it on every keystroke).
  const formRef = React.useRef(form);
  formRef.current = form;

  React.useEffect(() => {
    if (!definitions) return;
    const seeded = seedFormState(definitions, values);
    const seededJson = JSON.stringify(seeded);
    const currentJson = JSON.stringify(formRef.current);
    // Server state already matches the form: initial load, or a refetch that
    // caught up to a just-saved edit. Adopt it as the clean baseline (clears
    // the dirty flag) without a visible change.
    if (currentJson === seededJson) {
      baselineRef.current = seededJson;
      return;
    }
    // Form holds unsaved edits that differ from the incoming server state — a
    // background refetch or a mid-edit query invalidation. Do NOT clobber the
    // in-progress draft (the whole point of this hook is to prevent data loss).
    if (currentJson !== baselineRef.current) {
      return;
    }
    // Form is clean and the server values changed: adopt them.
    setForm(seeded);
    baselineRef.current = seededJson;
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
