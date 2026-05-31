import { describe, expect, it } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';

import {
  useCustomPropertyForm,
  coerceCustomProperty,
} from './use-custom-property-form';
import type { PropertyDefinition } from '@/lib/types';

function def(
  key: string,
  type: PropertyDefinition['property_type'],
  extra: Partial<PropertyDefinition> = {},
): PropertyDefinition {
  return {
    id: key,
    parent_type: 'deal',
    property_key: key,
    property_type: type,
    required: false,
    ...extra,
  };
}

const DEFS: PropertyDefinition[] = [
  def('source_du_lead', 'enum', { allowed_values: ['Salon', 'Web'] }),
  def('probabilite', 'number'),
  def('actif', 'boolean'),
];

// Stable references — the hook re-seeds whenever `definitions`/`values` change
// identity (matching how react-query hands back a stable object in the app),
// so tests must pass the same object across re-renders.
const VALUES_FULL = { source_du_lead: 'Salon', probabilite: 65 };
const VALUES_SOURCE = { source_du_lead: 'Salon' };

describe('coerceCustomProperty', () => {
  it('coerces by type and drops empty optional values', () => {
    expect(coerceCustomProperty(def('p', 'number'), '42')).toBe(42);
    expect(coerceCustomProperty(def('p', 'string'), 'hi')).toBe('hi');
    expect(coerceCustomProperty(def('p', 'boolean'), true)).toBe(true);
    expect(coerceCustomProperty(def('p', 'boolean'), false)).toBe(false);
    expect(coerceCustomProperty(def('p', 'string'), '')).toBeUndefined();
  });
});

describe('useCustomPropertyForm', () => {
  it('seeds from stored values and is not dirty until edited', async () => {
    const { result } = renderHook(() =>
      useCustomPropertyForm(DEFS, VALUES_FULL),
    );

    await waitFor(() => {
      expect(result.current.form.source_du_lead).toBe('Salon');
    });
    expect(result.current.form.probabilite).toBe('65');
    expect(result.current.form.actif).toBe(false);
    expect(result.current.isDirty).toBe(false);
  });

  it('tracks dirty state and builds a typed payload', async () => {
    const { result } = renderHook(() =>
      useCustomPropertyForm(DEFS, VALUES_FULL),
    );

    await waitFor(() => expect(result.current.form.source_du_lead).toBe('Salon'));

    act(() => result.current.set('probabilite', '80'));

    expect(result.current.isDirty).toBe(true);
    // Booleans are always emitted (false is meaningful); empty optionals drop.
    expect(result.current.buildPayload()).toEqual({
      source_du_lead: 'Salon',
      probabilite: 80,
      actif: false,
    });
  });

  it('returns to clean when an edit is reverted to the seeded value', async () => {
    const { result } = renderHook(() =>
      useCustomPropertyForm(DEFS, VALUES_SOURCE),
    );

    await waitFor(() => expect(result.current.form.source_du_lead).toBe('Salon'));

    act(() => result.current.set('source_du_lead', 'Web'));
    expect(result.current.isDirty).toBe(true);

    act(() => result.current.set('source_du_lead', 'Salon'));
    expect(result.current.isDirty).toBe(false);
  });

  it('is inert with no definitions', () => {
    const { result } = renderHook(() => useCustomPropertyForm(undefined, undefined));
    expect(result.current.isDirty).toBe(false);
    expect(result.current.buildPayload()).toEqual({});
  });
});
