import type { PropertyDefinition } from '@/lib/types';

// formatPropertyValue renders a stored custom-property value as a short display
// string for table cells, mirroring the type handling in
// custom-properties-editor.tsx (boolean, enum, number, date, json, string).
// Returns an empty string for absent values so callers can fall back to a dash.
export function formatPropertyValue(
  def: PropertyDefinition,
  value: unknown,
): string {
  if (value === undefined || value === null || value === '') return '';

  switch (def.property_type) {
    case 'boolean':
      return value === true || value === 'true' ? 'Yes' : 'No';
    case 'number': {
      const n = typeof value === 'number' ? value : Number(value);
      return Number.isNaN(n) ? String(value) : new Intl.NumberFormat().format(n);
    }
    case 'date': {
      const d = new Date(String(value));
      return Number.isNaN(d.getTime()) ? String(value) : d.toLocaleDateString();
    }
    case 'json':
      return typeof value === 'string' ? value : JSON.stringify(value);
    case 'enum':
    case 'string':
    default:
      return String(value);
  }
}
