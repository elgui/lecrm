import { describe, expect, it } from 'vitest';

import {
  BASELINE_DASHBOARDS,
  buildCubeFrameUrl,
  cubeEmbedBaseUrl,
  findDashboard,
  reportsEnabled,
} from './reports';

describe('BASELINE_DASHBOARDS', () => {
  it('ships the four v0 dashboards required by sprint-12', () => {
    expect(BASELINE_DASHBOARDS).toHaveLength(4);
    const ids = BASELINE_DASHBOARDS.map((d) => d.id).sort();
    expect(ids).toEqual([
      'conversion-funnel',
      'deals-by-owner',
      'deals-by-stage',
      'recent-activities',
    ]);
  });

  it('each dashboard has a query, title, and chart type', () => {
    for (const d of BASELINE_DASHBOARDS) {
      expect(d.title.length).toBeGreaterThan(0);
      expect(d.description.length).toBeGreaterThan(0);
      expect(d.query).toBeDefined();
      expect(['bar', 'line', 'funnel', 'table']).toContain(d.chartType);
    }
  });

  it('deals-by-stage groups counts by Deals.dealStage', () => {
    const d = findDashboard('deals-by-stage')!;
    expect(d.query.measures).toContain('Deals.count');
    expect(d.query.dimensions).toContain('Deals.dealStage');
  });

  it('deals-by-owner totals amount and caps at top 10', () => {
    const d = findDashboard('deals-by-owner')!;
    expect(d.query.measures).toContain('Deals.totalAmount');
    expect(d.query.measures).toContain('Deals.count');
    expect(d.query.limit).toBe(10);
  });

  it('recent-activities uses a 30-day time dimension', () => {
    const d = findDashboard('recent-activities')!;
    expect(d.query.measures).toContain('Activities.count');
    expect(d.query.timeDimensions?.[0]?.dateRange).toBe('last 30 days');
  });

  it('conversion-funnel uses funnel chart type over stages', () => {
    const d = findDashboard('conversion-funnel')!;
    expect(d.chartType).toBe('funnel');
    expect(d.query.dimensions).toContain('Deals.dealStage');
  });
});

describe('reportsEnabled', () => {
  it('is disabled when the flag is unset so the demo never hits the embed-token 503', () => {
    expect(reportsEnabled(undefined)).toBe(false);
  });

  it('is disabled with the real build-time env (unset on the demo)', () => {
    expect(reportsEnabled()).toBe(false);
  });

  it('only enables on the exact string "true"', () => {
    expect(reportsEnabled('true')).toBe(true);
  });

  it('treats any other truthy-looking value as disabled', () => {
    for (const v of ['1', 'yes', 'TRUE', '']) {
      expect(reportsEnabled(v)).toBe(false);
    }
  });
});

describe('cubeEmbedBaseUrl', () => {
  it('falls back to /cube/embed when VITE_CUBE_EMBED_URL is unset', () => {
    expect(cubeEmbedBaseUrl()).toBe('/cube/embed');
  });
});

describe('buildCubeFrameUrl', () => {
  it('puts the JWT in the query string and the dashboard id in the hash', () => {
    const url = buildCubeFrameUrl('abc.def.ghi', 'deals-by-stage', '/cube/embed');
    expect(url).toContain('token=abc.def.ghi');
    expect(url).toMatch(/#dashboard=deals-by-stage$/);
  });

  it('encodes dashboard ids with special characters', () => {
    const url = buildCubeFrameUrl('tok', 'a/b c', '/cube/embed');
    expect(url).toContain('#dashboard=a%2Fb%20c');
  });
});
