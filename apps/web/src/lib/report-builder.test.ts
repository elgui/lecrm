import { describe, it, expect } from 'vitest';
import {
  delta,
  deltaTone,
  formatDelta,
  formatMetricValue,
  parseDimension,
  dimensionOptions,
  definitionFrom,
  DEFAULT_DEFINITION,
  type SavedReport,
} from './report-builder';
import type { PropertyDefinition } from './types';

describe('delta', () => {
  it('computes a signed ratio against a positive baseline', () => {
    expect(delta(10, 5)).toBe(1); // +100%
    expect(delta(5, 10)).toBe(-0.5); // -50%
    expect(delta(7, 7)).toBe(0);
  });
  it('treats 0→0 as no change and 0→positive as no baseline', () => {
    expect(delta(0, 0)).toBe(0);
    expect(delta(3, 0)).toBeNull();
  });
});

describe('deltaTone', () => {
  it('classifies direction', () => {
    expect(deltaTone(0.2)).toBe('up');
    expect(deltaTone(-0.2)).toBe('down');
    expect(deltaTone(0)).toBe('flat');
    expect(deltaTone(null)).toBe('new');
  });
});

describe('formatDelta', () => {
  it('formats percentages with sign', () => {
    expect(formatDelta(0.25)).toBe('+25 %');
    expect(formatDelta(-0.5)).toBe('-50 %');
    expect(formatDelta(null)).toBe('nouveau');
  });
});

describe('formatMetricValue', () => {
  it('formats count as integer', () => {
    expect(formatMetricValue('deal_count', 12)).toBe('12');
  });
  it('formats win rate as percent (stored 0..1)', () => {
    expect(formatMetricValue('win_rate', 0.5)).toBe('50 %');
    expect(formatMetricValue('win_rate', 0.333)).toBe('33.3 %');
  });
  it('formats amount as EUR currency', () => {
    // Non-breaking spaces in fr-FR currency output — assert structure loosely.
    const s = formatMetricValue('deal_amount_sum', 8500);
    expect(s).toContain('8');
    expect(s).toContain('€');
  });
});

describe('parseDimension', () => {
  it('parses bare kinds', () => {
    expect(parseDimension('stage')).toEqual({ kind: 'stage', key: '' });
    expect(parseDimension('none')).toEqual({ kind: 'none', key: '' });
    expect(parseDimension('unknown')).toEqual({ kind: 'none', key: '' });
  });
  it('parses custom property dimensions', () => {
    expect(parseDimension('custom:source_du_lead')).toEqual({
      kind: 'custom',
      key: 'source_du_lead',
    });
  });
});

describe('dimensionOptions', () => {
  it('appends custom-property dimensions from definitions', () => {
    const defs: PropertyDefinition[] = [
      { id: '1', parent_type: 'deal', property_key: 'source_du_lead', property_type: 'enum', required: false },
    ];
    const opts = dimensionOptions(defs);
    const ids = opts.map((o) => o.id);
    expect(ids).toContain('stage');
    expect(ids).toContain('custom:source_du_lead');
  });
  it('returns base dimensions when no definitions', () => {
    expect(dimensionOptions(undefined).map((o) => o.id)).toEqual([
      'none',
      'stage',
      'owner',
      'company',
    ]);
  });
});

describe('definitionFrom', () => {
  it('merges a saved definition over defaults', () => {
    const saved: SavedReport = {
      id: 'x',
      definition: { name: 'R', metric: 'win_rate', dimension: 'owner', period: 'year', compare_yoy: true },
      created_at: '',
      updated_at: '',
    };
    expect(definitionFrom(saved)).toEqual({
      ...DEFAULT_DEFINITION,
      name: 'R',
      metric: 'win_rate',
      dimension: 'owner',
      period: 'year',
      compare_yoy: true,
    });
  });
});
