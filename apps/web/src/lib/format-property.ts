import type { PropertyDefinition } from '@/lib/types';
import { formatDate } from '@/lib/format';

// prettifyPropertyKey turns a snake_case property_key into a human-readable
// label using sentence case (capitalise the first letter only). This reads
// idiomatically for French field labels — `source_du_lead` → "Source du lead",
// `canal_signature` → "Canal signature" — instead of the DB-dump-looking key.
export function prettifyPropertyKey(key: string): string {
  const cleaned = key.replace(/[_\s]+/g, ' ').trim().toLowerCase();
  if (!cleaned) return key;
  return cleaned.charAt(0).toUpperCase() + cleaned.slice(1);
}

// customFieldLabel resolves the display label for a custom-field definition:
// the admin-set display_name when present, otherwise a prettified key. This is
// what every custom-property surface should render instead of the raw key.
export function customFieldLabel(def: PropertyDefinition): string {
  const displayName = def.display_name?.trim();
  if (displayName) return displayName;
  return prettifyPropertyKey(def.property_key);
}

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
      return value === true || value === 'true' ? 'Oui' : 'Non';
    case 'number': {
      const n = typeof value === 'number' ? value : Number(value);
      return Number.isNaN(n) ? String(value) : new Intl.NumberFormat('fr-FR').format(n);
    }
    case 'date':
      return formatDate(String(value));
    case 'json':
      return typeof value === 'string' ? value : JSON.stringify(value);
    case 'enum':
    case 'string':
    default:
      return String(value);
  }
}
