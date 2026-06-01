import { describe, expect, it } from 'vitest';

import { formatAmount, formatDateRelative, stageBadgeVariant } from './format';

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

describe('formatDateRelative', () => {
  // Fixed reference "now" (local midday so day-rounding is unambiguous).
  const now = new Date(2026, 4, 31, 12, 0, 0); // 31 May 2026

  it('returns an empty string for empty values', () => {
    expect(formatDateRelative(null, now)).toBe('');
    expect(formatDateRelative(undefined, now)).toBe('');
    expect(formatDateRelative('', now)).toBe('');
  });

  it('names today, tomorrow and yesterday in French', () => {
    expect(formatDateRelative('2026-05-31', now)).toBe("aujourd'hui");
    expect(formatDateRelative('2026-06-01', now)).toBe('demain');
    expect(formatDateRelative('2026-05-30', now)).toBe('hier');
  });

  it('counts forward and backward within the ±13 day window', () => {
    expect(formatDateRelative('2026-06-03', now)).toBe('dans 3 jours');
    expect(formatDateRelative('2026-06-13', now)).toBe('dans 13 jours');
    expect(formatDateRelative('2026-05-28', now)).toBe('il y a 3 jours');
    expect(formatDateRelative('2026-05-18', now)).toBe('il y a 13 jours');
  });

  it('falls back to a compact absolute date beyond the relative window', () => {
    // 14+ days out is no longer phrased relatively — assert it is NOT a
    // "dans … jours" string and carries the month abbreviation instead.
    const far = formatDateRelative('2026-07-13', now);
    expect(far).not.toMatch(/jours/);
    expect(far).toMatch(/juil/);
  });

  it('pins date-only values to local midnight (no off-by-one)', () => {
    // A late-evening "now" must still read the same calendar day as today.
    const lateNow = new Date(2026, 4, 31, 23, 30, 0);
    expect(formatDateRelative('2026-05-31', lateNow)).toBe("aujourd'hui");
  });
});
