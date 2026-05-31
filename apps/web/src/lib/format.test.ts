import { describe, expect, it } from 'vitest';

import { formatAmount, stageBadgeVariant } from './format';

describe('stageBadgeVariant', () => {
  it('maps the English gbconsult-default stages', () => {
    expect(stageBadgeVariant('Discovery')).toBe('secondary');
    expect(stageBadgeVariant('Qualified')).toBe('default');
    expect(stageBadgeVariant('Proposal Sent')).toBe('warning');
    expect(stageBadgeVariant('Negotiation')).toBe('warning');
    expect(stageBadgeVariant('Closed-Won/Lost')).toBe('success');
  });

  it('maps the French demo stages to the same colours', () => {
    // Découverte is the entry stage → coloured (default), unlike the legacy
    // grey "Discovery"; the rest mirror their English counterparts.
    expect(stageBadgeVariant('Découverte')).toBe('default');
    expect(stageBadgeVariant('Qualifié')).toBe('default');
    expect(stageBadgeVariant('Proposition envoyée')).toBe('warning');
    expect(stageBadgeVariant('Négociation')).toBe('warning');
    // Combined closed stage resolves to success (won wins over lost).
    expect(stageBadgeVariant('Gagné / Perdu')).toBe('success');
  });

  it('matches French stages even when accents are stripped', () => {
    expect(stageBadgeVariant('Decouverte')).toBe('default');
    expect(stageBadgeVariant('Qualifie')).toBe('default');
    expect(stageBadgeVariant('Negociation')).toBe('warning');
    expect(stageBadgeVariant('Gagne')).toBe('success');
    expect(stageBadgeVariant('Perdu')).toBe('destructive');
  });

  it('falls back to a neutral pill for unknown stages', () => {
    expect(stageBadgeVariant('Some Custom Stage')).toBe('secondary');
  });
});

describe('formatAmount', () => {
  it('renders an em dash for empty values', () => {
    expect(formatAmount(null)).toBe('—');
    expect(formatAmount(undefined)).toBe('—');
    expect(formatAmount('')).toBe('—');
  });

  it('formats numeric and string amounts in the fr-FR locale (EUR, space grouping)', () => {
    // fr-FR groups thousands with a (narrow) no-break space, not a comma, and
    // places the € symbol after the amount. Assert on the digits and currency
    // rather than the exact separator codepoint, which is ICU-version sensitive.
    const a = formatAmount(8500, 'EUR');
    expect(a.replace(/\D/g, '')).toBe('8500');
    expect(a).toContain('€');
    const b = formatAmount('14000', 'EUR');
    expect(b.replace(/\D/g, '')).toBe('14000');
    expect(b).toContain('€');
  });
});
