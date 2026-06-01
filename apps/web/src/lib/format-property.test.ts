import { describe, expect, it } from 'vitest';

import {
  formatPropertyValue,
  prettifyPropertyKey,
  customFieldLabel,
} from './format-property';
import type { PropertyDefinition, PropertyType } from '@/lib/types';

function def(type: PropertyType, allowed?: string[]): PropertyDefinition {
  return {
    id: '1',
    parent_type: 'deal',
    property_key: 'k',
    property_type: type,
    allowed_values: allowed,
    required: false,
  };
}

describe('formatPropertyValue', () => {
  it('renders booleans as Oui/Non (accepting string "true")', () => {
    expect(formatPropertyValue(def('boolean'), true)).toBe('Oui');
    expect(formatPropertyValue(def('boolean'), false)).toBe('Non');
    expect(formatPropertyValue(def('boolean'), 'true')).toBe('Oui');
  });

  it('formats numbers in the fr-FR locale, coercing strings', () => {
    expect(formatPropertyValue(def('number'), 42)).toBe(
      new Intl.NumberFormat('fr-FR').format(42),
    );
    expect(formatPropertyValue(def('number'), '1234.5')).toBe(
      new Intl.NumberFormat('fr-FR').format(1234.5),
    );
  });

  it('passes through enum and string values', () => {
    expect(formatPropertyValue(def('enum', ['salon', 'web']), 'salon')).toBe('salon');
    expect(formatPropertyValue(def('string'), 'hello')).toBe('hello');
  });

  it('stringifies json objects', () => {
    expect(formatPropertyValue(def('json'), { x: 1 })).toBe('{"x":1}');
    expect(formatPropertyValue(def('json'), '{"y":2}')).toBe('{"y":2}');
  });

  it('formats dates as a locale date string', () => {
    const out = formatPropertyValue(def('date'), '2026-05-30');
    expect(out).not.toBe('');
    expect(out).not.toBe('Invalid Date');
    // Must not echo the raw ISO string unchanged.
    expect(out).not.toBe('2026-05-30');
  });

  it('returns empty string for absent values', () => {
    expect(formatPropertyValue(def('string'), undefined)).toBe('');
    expect(formatPropertyValue(def('string'), null)).toBe('');
    expect(formatPropertyValue(def('string'), '')).toBe('');
  });
});

describe('prettifyPropertyKey', () => {
  it('sentence-cases snake_case keys (idiomatic for French labels)', () => {
    expect(prettifyPropertyKey('source_du_lead')).toBe('Source du lead');
    expect(prettifyPropertyKey('canal_signature')).toBe('Canal signature');
    expect(prettifyPropertyKey('probabilite')).toBe('Probabilite');
  });

  it('never echoes the raw snake_case key', () => {
    expect(prettifyPropertyKey('canal_prefere')).not.toContain('_');
    expect(prettifyPropertyKey('canal_prefere')).toBe('Canal prefere');
  });

  it('handles single words and stray separators', () => {
    expect(prettifyPropertyKey('tier')).toBe('Tier');
    expect(prettifyPropertyKey('account__tier')).toBe('Account tier');
  });
});

describe('customFieldLabel', () => {
  it('prefers an admin-set display_name when present', () => {
    const d: PropertyDefinition = { ...def('enum'), property_key: 'source_du_lead' };
    expect(customFieldLabel({ ...d, display_name: 'Source du lead' })).toBe(
      'Source du lead',
    );
  });

  it('falls back to a prettified key when display_name is missing or blank', () => {
    const d: PropertyDefinition = { ...def('string'), property_key: 'canal_prefere' };
    expect(customFieldLabel(d)).toBe('Canal prefere');
    expect(customFieldLabel({ ...d, display_name: '   ' })).toBe('Canal prefere');
    expect(customFieldLabel({ ...d, display_name: null })).toBe('Canal prefere');
  });
});
